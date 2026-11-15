package main

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func archiveFixture(t *testing.T, arch string, labels map[string]string, kind string) []byte {
	t.Helper()
	entries := map[string][]byte{}
	addBlob := func(data []byte, media string) descriptor {
		sum := fmt.Sprintf("%x", sha256.Sum256(data))
		entries["blobs/sha256/"+sum] = data
		return descriptor{MediaType: media, Digest: "sha256:" + sum, Size: int64(len(data))}
	}
	config, _ := json.Marshal(map[string]any{"architecture": arch, "os": "linux", "config": map[string]any{"Labels": labels}})
	c := addBlob(config, "application/vnd.oci.image.config.v1+json")
	layer := addBlob([]byte("fixture layer bytes"), "application/vnd.oci.image.layer.v1.tar")
	manifest := manifestDocument{SchemaVersion: 2, MediaType: "application/vnd.oci.image.manifest.v1+json", Config: c, Layers: []descriptor{layer}}
	m, _ := json.Marshal(manifest)
	d := addBlob(m, manifest.MediaType)
	index := indexDocument{SchemaVersion: 2, Manifests: []descriptor{d}}
	switch kind {
	case "attestation":
		index.Manifests = append(index.Manifests, d)
	case "nested":
		index.Manifests[0].MediaType = "application/vnd.oci.image.index.v1+json"
	case "tampered":
		entries["blobs/sha256/"+layer.Digest[7:]] = []byte("tampered layer bytes")
	case "missing":
		delete(entries, "blobs/sha256/"+layer.Digest[7:])
	}
	entries["index.json"], _ = json.Marshal(index)
	entries["oci-layout"] = []byte(`{"imageLayoutVersion":"1.0.0"}`)
	var buf bytes.Buffer
	w := tar.NewWriter(&buf)
	for name, data := range entries {
		if err := w.WriteHeader(&tar.Header{Name: name, Mode: 0644, Typeflag: tar.TypeReg, Size: int64(len(data))}); err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write(data); err != nil {
			t.Fatal(err)
		}
	}
	switch kind {
	case "symlink":
		if err := w.WriteHeader(&tar.Header{Name: "link", Typeflag: tar.TypeSymlink, Linkname: "/outside"}); err != nil {
			t.Fatal(err)
		}
	case "duplicate":
		if err := w.WriteHeader(&tar.Header{Name: "index.json", Typeflag: tar.TypeReg}); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestArchiveIntegrityAndNativeIdentity(t *testing.T) {
	for _, tc := range []struct {
		name, arch, kind string
		labels           map[string]string
		ok               bool
	}{
		{"valid", "amd64", "", map[string]string{"org.opencontainers.image.revision": "source"}, true},
		{"wrong-arch", "arm64", "", map[string]string{"org.opencontainers.image.revision": "source"}, false},
		{"missing-label", "amd64", "", nil, false},
		{"wrong-label", "amd64", "", map[string]string{"org.opencontainers.image.revision": "other"}, false},
		{"attestations", "amd64", "attestation", nil, false},
		{"nested-index", "amd64", "nested", nil, false},
		{"tampered-layer", "amd64", "tampered", nil, false},
		{"missing-layer", "amd64", "missing", nil, false},
		{"symlink-entry", "amd64", "symlink", nil, false},
		{"duplicate-entry", "amd64", "duplicate", nil, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			data := archiveFixture(t, tc.arch, tc.labels, tc.kind)
			p := filepath.Join(t.TempDir(), "image.tar")
			if err := os.WriteFile(p, data, 0600); err != nil {
				t.Fatal(err)
			}
			f, err := os.Open(p)
			if err != nil {
				t.Fatal(err)
			}
			defer f.Close()
			sum, size, err := verifyArchive(context.Background(), f, "amd64", map[string]string{"org.opencontainers.image.revision": "source"})
			if (err == nil) != tc.ok {
				t.Fatalf("expected success=%v, got %v", tc.ok, err)
			}
			if tc.ok && (sum != fmt.Sprintf("%x", sha256.Sum256(data)) || size != int64(len(data))) {
				t.Fatal("archive bytes were not hashed exactly")
			}
		})
	}
}

func TestArchiveCancellation(t *testing.T) {
	p := filepath.Join(t.TempDir(), "archive")
	if err := os.WriteFile(p, archiveFixture(t, "amd64", nil, ""), 0600); err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(p)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, err := verifyArchive(ctx, f, "amd64", nil); err == nil {
		t.Fatal("cancelled verification succeeded")
	}
}
