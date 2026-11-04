package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestArchivePreservesLayerAndRuntimeContract(t *testing.T) {
	root := t.TempDir()
	componentDir := filepath.Join(root, "component with spaces")
	os.Mkdir(componentDir, 0755)
	var tarBytes bytes.Buffer
	tw := tar.NewWriter(&tarBytes)
	payload := []byte("fixture executable\n")
	tw.WriteHeader(&tar.Header{Name: "usr/bin/fixture", Mode: 0755, Size: int64(len(payload))})
	tw.Write(payload)
	tw.Close()
	var compressed bytes.Buffer
	gz := gzip.NewWriter(&compressed)
	gz.Write(tarBytes.Bytes())
	gz.Close()
	hash := func(data []byte) string { value := sha256.Sum256(data); return hex.EncodeToString(value[:]) }
	source := map[string]any{"repository": "fixture/core", "commit": strings.Repeat("a", 40), "dirty": true, "treeSha256": strings.Repeat("b", 64)}
	entrypoint := []string{"/fixture/init", "--literal", "argument with spaces"}
	encode := func(value any) []byte {
		data, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		return data
	}
	files := map[string][]byte{
		"rootfs.tar.gz": compressed.Bytes(), "rootfs.squashfs": []byte("squashfs fixture"), "packages.manifest": []byte("fixture"), "lxc.conf.in": []byte("fixture"), "sbom.spdx.tar.zst": []byte("fixture"),
		"runtime.json":          encode(map[string]any{"schema": 1, "id": "pantavisor-core-v1", "architecture": "arm64", "requires": []string{}, "provides": []string{}, "entrypoint": entrypoint}),
		"build-provenance.json": encode(map[string]any{"schema": 1, "source": source}),
	}
	inventory := map[string]any{}
	for name, data := range files {
		if err := os.WriteFile(filepath.Join(componentDir, name), data, 0644); err != nil {
			t.Fatal(err)
		}
		inventory[name] = map[string]any{"size": len(data), "sha256": hash(data)}
	}
	metadata := map[string]any{"schema": 2, "name": "core", "kind": "rootfs", "version": "0.1.0", "architecture": "arm64", "features": []string{}, "provides": []string{}, "requires": []string{}, "runtimeABI": "pantavisor-core-v1", "source": source, "files": inventory, "rootfsSha256": hash(files["rootfs.squashfs"])}
	os.WriteFile(filepath.Join(componentDir, "component.json"), encode(metadata), 0644)
	output := filepath.Join(root, "image.oci.tar")
	oldFlags, oldArgs := flag.CommandLine, os.Args
	t.Cleanup(func() { flag.CommandLine = oldFlags; os.Args = oldArgs })
	t.Setenv("GITHUB_OUTPUT", "")
	invoke := func() error {
		flag.CommandLine = flag.NewFlagSet("fixture", flag.ContinueOnError)
		os.Args = []string{"fixture", "--directory=" + componentDir, "--output=" + output}
		return run()
	}
	if err := invoke(); err != nil {
		t.Fatal(err)
	}
	archive, err := os.Open(output)
	if err != nil {
		t.Fatal(err)
	}
	defer archive.Close()
	reader := tar.NewReader(archive)
	members := map[string][]byte{}
	for {
		header, err := reader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		data, err := io.ReadAll(reader)
		if err != nil {
			t.Fatal(err)
		}
		members[header.Name] = data
	}
	var index struct {
		Manifests []descriptor `json:"manifests"`
	}
	json.Unmarshal(members["index.json"], &index)
	if len(index.Manifests) != 1 {
		t.Fatal("incorrect OCI index")
	}
	manifestBytes := members["blobs/sha256/"+strings.TrimPrefix(index.Manifests[0].Digest, "sha256:")]
	if "sha256:"+hash(manifestBytes) != index.Manifests[0].Digest {
		t.Fatal("manifest identity mismatch")
	}
	var manifest struct {
		Config descriptor   `json:"config"`
		Layers []descriptor `json:"layers"`
	}
	json.Unmarshal(manifestBytes, &manifest)
	if len(manifest.Layers) != 1 || manifest.Layers[0].Digest != "sha256:"+hash(compressed.Bytes()) || !bytes.Equal(members["blobs/sha256/"+hash(compressed.Bytes())], compressed.Bytes()) {
		t.Fatal("compressed layer was changed")
	}
	var configuration struct {
		Architecture string `json:"architecture"`
		Config       struct {
			Entrypoint []string `json:"Entrypoint"`
			User       string   `json:"User"`
		} `json:"config"`
		Rootfs struct {
			DiffIDs []string `json:"diff_ids"`
		} `json:"rootfs"`
	}
	json.Unmarshal(members["blobs/sha256/"+strings.TrimPrefix(manifest.Config.Digest, "sha256:")], &configuration)
	if configuration.Architecture != "arm64" || configuration.Config.User != "0:0" || !reflect.DeepEqual(configuration.Config.Entrypoint, entrypoint) || !reflect.DeepEqual(configuration.Rootfs.DiffIDs, []string{"sha256:" + hash(tarBytes.Bytes())}) {
		t.Fatal("runtime or diff ID contract changed")
	}
	before, _ := os.ReadFile(output)
	if err := invoke(); err == nil {
		t.Fatal("existing archive overwritten")
	}
	after, _ := os.ReadFile(output)
	if !bytes.Equal(before, after) {
		t.Fatal("failed retry changed the archive")
	}
}
