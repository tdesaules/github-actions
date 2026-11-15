package main

import (
	"archive/tar"
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestAtomicFixturePreservesFilesystemLayers(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source.tar")
	if err := archive(source, runtime.GOARCH); err != nil {
		t.Fatal(err)
	}
	members := func(path string) map[string][]byte {
		t.Helper()
		f, err := os.Open(path)
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		tr := tar.NewReader(f)
		result := map[string][]byte{}
		for {
			h, err := tr.Next()
			if err == io.EOF {
				return result
			}
			if err != nil {
				t.Fatal(err)
			}
			result[h.Name], err = io.ReadAll(tr)
			if err != nil {
				t.Fatal(err)
			}
		}
	}
	original := members(source)
	oldCopy := copyAtomicImage
	copyAtomicImage = func(_ string, layout string) error {
		for name, data := range original {
			path := filepath.Join(layout, name)
			if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
				return err
			}
			if err := os.WriteFile(path, data, 0644); err != nil {
				return err
			}
		}
		return nil
	}
	t.Cleanup(func() { copyAtomicImage = oldCopy })
	recipe := filepath.Join(root, "Containerfile")
	base := "FROM ghcr.io/fixture/os@sha256:" + strings.Repeat("a", 64) + " AS final\n"
	if err := os.WriteFile(recipe, []byte(base), 0644); err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(root, "fixture.tar")
	revision := strings.Repeat("b", 40)
	sum, err := atomicFixture(recipe, output, revision)
	if err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(output)
	if sum != fmt.Sprintf("%x", sha256.Sum256(data)) {
		t.Fatal("incorrect exported checksum")
	}
	result := members(output)
	for name, data := range original {
		if strings.HasPrefix(name, "blobs/") && !bytes.Equal(data, result[name]) {
			t.Fatalf("original content was modified: %s", name)
		}
	}
	var index struct {
		Manifests []atomicDescriptor `json:"manifests"`
	}
	if err := json.Unmarshal(result["index.json"], &index); err != nil {
		t.Fatal(err)
	}
	var manifest struct {
		Config atomicDescriptor `json:"config"`
	}
	if err := json.Unmarshal(result["blobs/sha256/"+index.Manifests[0].Digest[7:]], &manifest); err != nil {
		t.Fatal(err)
	}
	var config struct {
		Config struct {
			Labels map[string]string `json:"Labels"`
		} `json:"config"`
	}
	if err := json.Unmarshal(result["blobs/sha256/"+manifest.Config.Digest[7:]], &config); err != nil {
		t.Fatal(err)
	}
	if config.Config.Labels["org.opencontainers.image.revision"] != revision {
		t.Fatal("missing fixture revision")
	}
	if err := os.WriteFile(recipe, []byte(base+"RUN touch /changed\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := atomicFixture(recipe, filepath.Join(root, "invalid.tar"), revision); err == nil {
		t.Fatal("silently ignored a filesystem mutation")
	}
}
