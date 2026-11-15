package main

import (
	"archive/tar"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
)

type atomicDescriptor struct {
	MediaType string `json:"mediaType"`
	Digest    string `json:"digest"`
	Size      int64  `json:"size"`
}

var copyAtomicImage = func(reference, layout string) error {
	cmd := exec.Command("skopeo", "--override-arch", runtime.GOARCH, "copy", "docker://"+reference, "oci:"+layout+":fixture")
	cmd.Stdout, cmd.Stderr = os.Stderr, os.Stderr
	return cmd.Run()
}

// Preserve the real consumer's compressed filesystem layers. Buildah's VFS layer
// snapshots exhaust hosted disk before the FROM-only fixture can be exported.
// Only the synthetic fixture's revision label changes, not consumer file content.
func atomicFixture(recipe, output, revision string) (string, error) {
	if !regexp.MustCompile(`^[a-f0-9]{40}$`).MatchString(revision) {
		return "", fmt.Errorf("fixture source revision required")
	}
	data, err := os.ReadFile(recipe)
	if err != nil {
		return "", err
	}
	reference := ""
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "LABEL ") {
			continue
		}
		match := regexp.MustCompile(`^FROM (ghcr\.io/[a-z0-9/_.-]+@sha256:[a-f0-9]{64}) AS final$`).FindStringSubmatch(line)
		if match == nil || reference != "" {
			return "", fmt.Errorf("Atomic fixture must be a single digest-pinned FROM-only recipe")
		}
		reference = match[1]
	}
	if reference == "" {
		return "", fmt.Errorf("missing fixture reference")
	}
	layout, err := os.MkdirTemp("", "atomic-fixture-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(layout)
	if err := copyAtomicImage(reference, layout); err != nil {
		return "", err
	}
	readBlob := func(d atomicDescriptor) ([]byte, error) {
		if !regexp.MustCompile(`^sha256:[a-f0-9]{64}$`).MatchString(d.Digest) {
			return nil, fmt.Errorf("invalid blob digest")
		}
		data, err := os.ReadFile(filepath.Join(layout, "blobs/sha256", d.Digest[7:]))
		if err != nil {
			return nil, err
		}
		if int64(len(data)) != d.Size || fmt.Sprintf("sha256:%x", sha256.Sum256(data)) != d.Digest {
			return nil, fmt.Errorf("fixture metadata digest mismatch")
		}
		return data, nil
	}
	writeBlob := func(v any, media string) (atomicDescriptor, error) {
		data, err := json.Marshal(v)
		if err != nil {
			return atomicDescriptor{}, err
		}
		d := atomicDescriptor{media, fmt.Sprintf("sha256:%x", sha256.Sum256(data)), int64(len(data))}
		return d, os.WriteFile(filepath.Join(layout, "blobs/sha256", d.Digest[7:]), data, 0644)
	}
	var index struct {
		SchemaVersion int                `json:"schemaVersion"`
		Manifests     []atomicDescriptor `json:"manifests"`
	}
	data, err = os.ReadFile(filepath.Join(layout, "index.json"))
	if err != nil {
		return "", err
	}
	if err := json.Unmarshal(data, &index); err != nil || len(index.Manifests) != 1 {
		return "", fmt.Errorf("fixture needs one native manifest")
	}
	data, err = readBlob(index.Manifests[0])
	if err != nil {
		return "", err
	}
	var manifest map[string]json.RawMessage
	if err := json.Unmarshal(data, &manifest); err != nil {
		return "", err
	}
	var configDescriptor atomicDescriptor
	if err := json.Unmarshal(manifest["config"], &configDescriptor); err != nil {
		return "", err
	}
	data, err = readBlob(configDescriptor)
	if err != nil {
		return "", err
	}
	var config map[string]json.RawMessage
	if err := json.Unmarshal(data, &config); err != nil {
		return "", err
	}
	var arch, osName string
	if json.Unmarshal(config["architecture"], &arch) != nil || arch != runtime.GOARCH ||
		json.Unmarshal(config["os"], &osName) != nil || osName != "linux" {
		return "", fmt.Errorf("fixture platform mismatch")
	}
	var settings map[string]json.RawMessage
	if err := json.Unmarshal(config["config"], &settings); err != nil {
		return "", err
	}
	if settings == nil {
		settings = map[string]json.RawMessage{}
	}
	labels := map[string]string{}
	if raw := settings["Labels"]; len(raw) > 0 {
		if err := json.Unmarshal(raw, &labels); err != nil {
			return "", err
		}
	}
	if labels == nil {
		labels = map[string]string{}
	}
	labels["org.opencontainers.image.revision"] = revision
	settings["Labels"], _ = json.Marshal(labels)
	config["config"], _ = json.Marshal(settings)
	configDescriptor, err = writeBlob(config, "application/vnd.oci.image.config.v1+json")
	if err != nil {
		return "", err
	}
	manifest["config"], _ = json.Marshal(configDescriptor)
	index.Manifests[0], err = writeBlob(manifest, "application/vnd.oci.image.manifest.v1+json")
	if err != nil {
		return "", err
	}
	data, err = json.Marshal(index)
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(layout, "index.json"), data, 0644); err != nil {
		return "", err
	}
	f, err := os.OpenFile(output, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
	if err != nil {
		return "", err
	}
	hash := sha256.New()
	w := tar.NewWriter(io.MultiWriter(f, hash))
	err = filepath.WalkDir(layout, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("nonregular fixture member")
		}
		name, err := filepath.Rel(layout, path)
		if err != nil {
			return err
		}
		if err := w.WriteHeader(&tar.Header{Name: filepath.ToSlash(name), Mode: 0644, Size: info.Size(), Typeflag: tar.TypeReg}); err != nil {
			return err
		}
		member, err := os.Open(path)
		if err != nil {
			return err
		}
		_, err = io.Copy(w, member)
		member.Close()
		return err
	})
	closeTarErr, closeFileErr := w.Close(), f.Close()
	if err != nil || closeTarErr != nil || closeFileErr != nil {
		os.Remove(output)
		return "", fmt.Errorf("writing fixture archive: %v %v %v", err, closeTarErr, closeFileErr)
	}
	return fmt.Sprintf("%x", hash.Sum(nil)), nil
}
