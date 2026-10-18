package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

type descriptor struct {
	Digest string `json:"digest"`
	Size   int64  `json:"size"`
}

func layer(path string, d descriptor, mode *int64) error {
	if !regexp.MustCompile(`^sha256:[a-f0-9]{64}$`).MatchString(d.Digest) {
		return errors.New("invalid layer digest")
	}
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	archive := tar.NewReader(f)
	for {
		header, err := archive.Next()
		if err != nil {
			return err
		}
		if header.Name != "blobs/sha256/"+d.Digest[7:] {
			continue
		}
		if header.Size != d.Size {
			return errors.New("layer size mismatch")
		}
		hash := sha256.New()
		data := io.TeeReader(archive, hash)
		prefix := make([]byte, 2)
		if _, err = io.ReadFull(data, prefix); err != nil {
			return err
		}
		var content io.Reader = io.MultiReader(bytes.NewReader(prefix), data)
		if prefix[0] == 0x1f && prefix[1] == 0x8b {
			compressed, err := gzip.NewReader(content)
			if err != nil {
				return err
			}
			defer compressed.Close()
			content = compressed
		}
		entries := tar.NewReader(content)
		for {
			entry, err := entries.Next()
			if errors.Is(err, io.EOF) {
				break
			}
			if err != nil {
				return err
			}
			name := strings.TrimSuffix(strings.TrimPrefix(entry.Name, "./"), "/")
			if name == ".wh.tmp" {
				*mode = 0
			}
			if name == "tmp" && entry.Typeflag == tar.TypeDir {
				*mode = entry.Mode
			}
		}
		if _, err = io.Copy(io.Discard, content); err != nil {
			return err
		}
		if _, err = io.Copy(io.Discard, data); err != nil {
			return err
		}
		if "sha256:"+hex.EncodeToString(hash.Sum(nil)) != d.Digest {
			return errors.New("layer checksum mismatch")
		}
		return nil
	}
}

func member(path, name string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	reader := tar.NewReader(f)
	for {
		header, err := reader.Next()
		if err != nil {
			return nil, err
		}
		if header.Name != name {
			continue
		}
		if header.Typeflag != tar.TypeReg || header.Size > 16<<20 {
			return nil, errors.New("invalid or oversized OCI metadata member")
		}
		return io.ReadAll(reader)
	}
}

func blob(path string, d descriptor) ([]byte, error) {
	if !regexp.MustCompile(`^sha256:[a-f0-9]{64}$`).MatchString(d.Digest) {
		return nil, errors.New("invalid OCI digest")
	}
	data, err := member(path, "blobs/sha256/"+d.Digest[7:])
	if err != nil {
		return nil, err
	}
	if int64(len(data)) != d.Size || fmt.Sprintf("sha256:%x", sha256.Sum256(data)) != d.Digest {
		return nil, errors.New("OCI metadata checksum mismatch")
	}
	return data, nil
}

func inspect(path, arch string) (string, error) {
	data, err := member(path, "index.json")
	if err != nil {
		return "", err
	}
	var index struct {
		Manifests []descriptor `json:"manifests"`
	}
	if err = json.Unmarshal(data, &index); err != nil {
		return "", err
	}
	if len(index.Manifests) != 1 {
		return "", errors.New("archive must contain exactly one platform")
	}
	data, err = blob(path, index.Manifests[0])
	if err != nil {
		return "", err
	}
	var manifest struct {
		Config descriptor   `json:"config"`
		Layers []descriptor `json:"layers"`
	}
	if err = json.Unmarshal(data, &manifest); err != nil {
		return "", err
	}
	data, err = blob(path, manifest.Config)
	if err != nil {
		return "", err
	}
	var config struct {
		OS           string `json:"os"`
		Architecture string `json:"architecture"`
	}
	if err = json.Unmarshal(data, &config); err != nil {
		return "", err
	}
	if config.OS != "linux" || config.Architecture != arch {
		return "", errors.New("archive platform does not match the tested platform")
	}
	var temporaryMode int64
	for _, d := range manifest.Layers {
		if err = layer(path, d, &temporaryMode); err != nil {
			return "", err
		}
	}
	if temporaryMode != 01777 {
		return "", fmt.Errorf("archive /tmp must be 01777, got %o", temporaryMode)
	}
	return index.Manifests[0].Digest, nil
}

func execute(args []string) error {
	flags := flag.NewFlagSet("new-image-metadata", flag.ContinueOnError)
	archive := flags.String("archive", os.Getenv("INPUT_ARCHIVE"), "tested OCI archive")
	image := flags.String("image", os.Getenv("INPUT_IMAGE"), "component image name")
	arch := flags.String("architecture", os.Getenv("INPUT_ARCHITECTURE"), "native architecture")
	revision := flags.String("revision", os.Getenv("INPUT_REVISION"), "tested source commit")
	version := flags.String("version", os.Getenv("INPUT_VERSION"), "candidate version")
	output := flags.String("output", os.Getenv("INPUT_OUTPUT"), "metadata JSON path")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 || *archive == "" || *output == "" || *version == "" || !regexp.MustCompile(`^[a-z0-9][a-z0-9-]+$`).MatchString(*image) || !regexp.MustCompile(`^[a-f0-9]{40}$`).MatchString(*revision) || (*arch != "amd64" && *arch != "arm64") {
		return errors.New("archive, image, architecture, revision, version and output are required")
	}
	source, _ := filepath.Abs(*archive)
	target, _ := filepath.Abs(*output)
	if source == target {
		return errors.New("metadata output must not overwrite the archive")
	}
	digest, err := inspect(*archive, *arch)
	if err != nil {
		return err
	}
	f, err := os.Open(*archive)
	if err != nil {
		return err
	}
	hash := sha256.New()
	_, err = io.Copy(hash, f)
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	data, err := json.MarshalIndent(map[string]any{"image": *image, "arch": *arch, "version": *version, "revision": *revision, "archive_sha256": hex.EncodeToString(hash.Sum(nil)), "digest": digest, "tested": true}, "", "  ")
	if err != nil {
		return err
	}
	if err = os.MkdirAll(filepath.Dir(*output), 0755); err != nil {
		return err
	}
	return os.WriteFile(*output, append(data, '\n'), 0644)
}

func main() {
	if len(os.Args) == 2 && os.Args[1] == "--tool-version" {
		fmt.Println("new-image-metadata 1")
		return
	}
	if err := execute(os.Args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return
		}
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
