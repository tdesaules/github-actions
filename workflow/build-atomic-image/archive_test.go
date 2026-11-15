package main

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
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
	case "empty-index":
		index.Manifests = nil
	case "bad-digest":
		index.Manifests[0].Digest = "sha256:" + strings.Repeat("g", 64)
	case "missing-manifest":
		index.Manifests[0].Digest = "sha256:" + strings.Repeat("0", 64)
	case "wrong-algorithm":
		index.Manifests[0].Digest = "sha512:" + strings.Repeat("0", 64)
	case "config-descriptor":
		index.Manifests[0].Digest = c.Digest
		index.Manifests[0].Size = c.Size
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
		{"empty-index", "amd64", "empty-index", nil, false},
		{"bad-digest", "amd64", "bad-digest", nil, false},
		{"missing-manifest", "amd64", "missing-manifest", nil, false},
		{"wrong-algorithm", "amd64", "wrong-algorithm", nil, false},
		{"config-descriptor", "amd64", "config-descriptor", nil, false},
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
			r, err := verifyArchive(context.Background(), f, "amd64", map[string]string{"org.opencontainers.image.revision": "source"})
			if (err == nil) != tc.ok {
				t.Fatalf("expected success=%v, got %v", tc.ok, err)
			}
			if tc.ok && (r.SHA256 != fmt.Sprintf("%x", sha256.Sum256(data)) || r.Size != int64(len(data))) {
				t.Fatal("archive bytes were not hashed exactly")
			}
			if tc.ok {
				native, config := fixtureManifestDigests(t, data)
				if r.ManifestDigest != native || r.ManifestDigest == config {
					t.Fatal("returned digest is not the native manifest identity")
				}
			} else if r.ManifestDigest != "" {
				t.Fatal("failed verification exposed an unverified manifest digest")
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
	if _, err := verifyArchive(ctx, f, "amd64", nil); err == nil {
		t.Fatal("cancelled verification succeeded")
	}
}

// Identify fixture documents by their JSON structure, independently of the
// index descriptor, then hash their actual bytes. Config and manifest identities
// must remain distinct in constructor outputs.
func fixtureManifestDigests(t *testing.T, data []byte) (string, string) {
	t.Helper()
	reader := tar.NewReader(bytes.NewReader(data))
	var manifestDigest, configDigest string
	for {
		header, err := reader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		body, err := io.ReadAll(reader)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.HasPrefix(header.Name, "blobs/sha256/") {
			continue
		}
		var document map[string]any
		if json.Unmarshal(body, &document) != nil {
			continue
		}
		sum := fmt.Sprintf("sha256:%x", sha256.Sum256(body))
		if document["mediaType"] == "application/vnd.oci.image.manifest.v1+json" {
			manifestDigest = sum
		}
		if document["architecture"] != nil {
			configDigest = sum
		}
	}
	if manifestDigest == "" || configDigest == "" || manifestDigest == configDigest {
		t.Fatal("fixture must have distinct native manifest and configuration")
	}
	return manifestDigest, configDigest
}
