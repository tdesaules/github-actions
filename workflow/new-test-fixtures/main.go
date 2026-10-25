package main

import (
	"archive/tar"
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"

	"github.com/tdesaules/github-actions/shared/fixtures"
)

var failureBinary = "/fixture-fail-run"
var successBinary = "/fixture-success-run"

func archive(path, arch string) error {
	var layer bytes.Buffer
	w := tar.NewWriter(&layer)
	if err := w.WriteHeader(&tar.Header{Name: "tmp", Mode: 01777, Typeflag: tar.TypeDir}); err != nil {
		return err
	}
	if err := w.WriteHeader(&tar.Header{Name: "hello.txt", Mode: 0644, Size: 6, Typeflag: tar.TypeReg}); err != nil {
		return err
	}
	if _, err := w.Write([]byte("hello\n")); err != nil {
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	blobs := map[string][]byte{}
	add := func(data []byte, media string) map[string]any {
		digest := fmt.Sprintf("sha256:%x", sha256.Sum256(data))
		blobs["blobs/sha256/"+digest[7:]] = data
		return map[string]any{"digest": digest, "size": len(data), "mediaType": media}
	}
	l := add(layer.Bytes(), "application/vnd.oci.image.layer.v1.tar")
	config, _ := json.Marshal(map[string]any{"architecture": arch, "os": "linux", "config": map[string]any{}, "rootfs": map[string]any{"type": "layers", "diff_ids": []any{l["digest"]}}})
	c := add(config, "application/vnd.oci.image.config.v1+json")
	manifest, _ := json.Marshal(map[string]any{"schemaVersion": 2, "mediaType": "application/vnd.oci.image.manifest.v1+json", "config": c, "layers": []any{l}})
	m := add(manifest, "application/vnd.oci.image.manifest.v1+json")
	m["annotations"] = map[string]string{"org.opencontainers.image.ref.name": "fixture"}
	blobs["index.json"], _ = json.Marshal(map[string]any{"schemaVersion": 2, "mediaType": "application/vnd.oci.image.index.v1+json", "manifests": []any{m}})
	blobs["oci-layout"] = []byte(`{"imageLayoutVersion":"1.0.0"}`)
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	w = tar.NewWriter(f)
	var names []string
	for name := range blobs {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		data := blobs[name]
		if err = w.WriteHeader(&tar.Header{Name: name, Mode: 0644, Size: int64(len(data)), Typeflag: tar.TypeReg}); err != nil {
			return err
		}
		if _, err = w.Write(data); err != nil {
			return err
		}
	}
	return w.Close()
}

func execute(args []string) error {
	flags := flag.NewFlagSet("new-test-fixtures", flag.ContinueOnError)
	directory := flags.String("directory", os.Getenv("INPUT_DIRECTORY"), "fixture directory")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 || *directory == "" {
		return errors.New("fixture directory is required")
	}
	if output := os.Getenv("GITHUB_OUTPUT"); output != "" && os.Getuid() == 0 {
		info, err := os.Stat(output)
		if err != nil {
			return err
		}
		stat := info.Sys().(*syscall.Stat_t)
		if stat.Uid != 0 {
			if err = syscall.Setgroups([]int{}); err != nil {
				return err
			}
			if err = syscall.Setgid(int(stat.Gid)); err != nil {
				return err
			}
			if err = syscall.Setuid(int(stat.Uid)); err != nil {
				return err
			}
		}
	}
	files := fixtures.Files()
	files["fixture with spaces/Containerfile"] = "FROM scratch AS build\nCOPY success-run /success-run\nRUN [\"/success-run\"]\nFROM scratch AS final\nCOPY --from=build /runtime/ /\nCOPY hello.txt /hello.txt\nUSER 65532:65532\n"
	files["fixture with spaces/Containerfile.fail"] = "FROM scratch\nCOPY fail-run /fail-run\nRUN [\"/fail-run\"]\n"
	for name, data := range files {
		path := filepath.Join(*directory, name)
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			return err
		}
		if err := os.WriteFile(path, []byte(data), 0644); err != nil {
			return err
		}
	}
	if err := archive(filepath.Join(*directory, "fixture.oci.tar"), "amd64"); err != nil {
		return err
	}
	if err := archive(filepath.Join(*directory, "arm64.oci.tar"), "arm64"); err != nil {
		return err
	}
	fail, err := os.ReadFile(failureBinary)
	if err != nil {
		return err
	}
	if err = os.WriteFile(filepath.Join(*directory, "fixture with spaces/fail-run"), fail, 0755); err != nil {
		return err
	}
	success, err := os.ReadFile(successBinary)
	if err != nil {
		return err
	}
	if err = os.WriteFile(filepath.Join(*directory, "fixture with spaces/success-run"), success, 0755); err != nil {
		return err
	}
	checksums := map[string]string{}
	for _, name := range []string{"fixture.oci.tar", "arm64.oci.tar"} {
		data, err := os.ReadFile(filepath.Join(*directory, name))
		if err != nil {
			return err
		}
		checksums[name] = fmt.Sprintf("%x", sha256.Sum256(data))
	}
	data, err := json.Marshal(checksums)
	if err != nil {
		return err
	}
	if err = os.WriteFile(filepath.Join(*directory, "fixture-checksums.json"), data, 0644); err != nil {
		return err
	}
	data, err = json.Marshal(map[string]any{"image": "fixture", "arch": "amd64", "revision": strings.Repeat("0", 40), "version": "fixture", "archive_sha256": checksums["fixture.oci.tar"], "tested": false})
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(*directory, "fixture-build.json"), data, 0644)
}

func main() {
	if len(os.Args) == 2 && os.Args[1] == "--tool-version" {
		fmt.Println("new-test-fixtures 1")
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
