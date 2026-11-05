package review

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type layerEntry struct {
	name, body, link string
	kind             byte
}

func layerBytes(t *testing.T, entries ...layerEntry) []byte {
	t.Helper()
	var buffer bytes.Buffer
	w := tar.NewWriter(&buffer)
	for _, entry := range entries {
		kind := entry.kind
		if kind == 0 {
			kind = tar.TypeReg
		}
		h := &tar.Header{Name: entry.name, Mode: 0644, Typeflag: kind, Linkname: entry.link, Size: int64(len(entry.body))}
		if err := w.WriteHeader(h); err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(entry.body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

func ociFixture(t *testing.T, layers ...[]byte) (string, SBOM) {
	t.Helper()
	s, _, _ := applicabilityFixture(t)
	files := map[string][]byte{}
	put := func(data []byte, media string) archiveDescriptor {
		digest := fmt.Sprintf("sha256:%x", sha256.Sum256(data))
		files["blobs/sha256/"+strings.TrimPrefix(digest, "sha256:")] = data
		return archiveDescriptor{Digest: digest, Size: int64(len(data)), MediaType: media}
	}
	config := put([]byte(`{"architecture":"amd64","os":"linux","config":{"Labels":{"org.opencontainers.image.title":"workflow-build-pantavisor"}}}`), "application/vnd.oci.image.config.v1+json")
	descriptors := []archiveDescriptor{}
	for _, layer := range layers {
		media := "application/vnd.oci.image.layer.v1.tar"
		if bytes.HasPrefix(layer, []byte{0x1f, 0x8b}) {
			media += "+gzip"
		}
		descriptors = append(descriptors, put(layer, media))
	}
	manifest, _ := json.Marshal(map[string]any{"schemaVersion": 2, "config": config, "layers": descriptors})
	md := put(manifest, "application/vnd.oci.image.manifest.v1+json")
	files["index.json"], _ = json.Marshal(map[string]any{"schemaVersion": 2, "manifests": []archiveDescriptor{md}})
	files["oci-layout"] = []byte(`{"imageLayoutVersion":"1.0.0"}`)
	entries := []layerEntry{}
	for name, body := range files {
		entries = append(entries, layerEntry{name: name, body: string(body)})
	}
	filename := filepath.Join(t.TempDir(), "image.oci.tar")
	if err := os.WriteFile(filename, layerBytes(t, entries...), 0600); err != nil {
		t.Fatal(err)
	}
	s.Source.Metadata.ManifestDigest = md.Digest
	s.Source.Metadata.ImageID = config.Digest
	s.Inventory = nil
	return filename, s
}

func TestOCIInventoryDetectsComponentHiddenBySyftExclusion(t *testing.T) {
	layer := layerBytes(t, layerEntry{name: "usr/lib/libexample.so", body: "payload"}, layerEntry{name: "usr/bin/missing-tool", body: "present"})
	filename, s := ociFixture(t, layer)
	_, p, now := applicabilityFixture(t)
	hash := fmt.Sprintf("%x", sha256.Sum256([]byte("payload")))
	s.Files[0].Digests[0].Value = hash
	p.Variants["amd64"].Files["/usr/lib/libexample.so"] = hash
	var err error
	s.Inventory, err = ReadInventory(filename, s)
	if err != nil {
		t.Fatal(err)
	}
	fingerprint, err := s.Inventory.Fingerprint()
	if err != nil {
		t.Fatal(err)
	}
	variant := p.Variants["amd64"]
	variant.InventorySHA256 = fingerprint
	p.Variants["amd64"] = variant
	rules, _ := applicabilityReview(s, p, now)
	if len(rules) != 0 {
		t.Fatal("a filtered SBOM concealed an archive component")
	}
}

func TestOCIInventoryWhiteoutsAndHardLinks(t *testing.T) {
	old := layerBytes(t, layerEntry{name: "usr/bin/missing-tool", body: "present"}, layerEntry{name: "usr/lib/libexample.so", body: "old"})
	next := layerBytes(t,
		layerEntry{name: "usr/bin/kept", body: "kept"},
		layerEntry{name: "usr/bin/.wh..wh..opq"},
		layerEntry{name: "usr/lib/alias.so", kind: tar.TypeLink, link: "usr/lib/libexample.so"},
		layerEntry{name: "usr/lib/libexample.so", body: "new"})
	filename, s := ociFixture(t, old, next)
	inv, err := ReadInventory(filename, s)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := inv.Files["/usr/bin/missing-tool"]; ok {
		t.Fatal("opaque directory retained a lower-layer file")
	}
	if _, ok := inv.Files["/usr/bin/kept"]; !ok {
		t.Fatal("opaque directory discarded a same-layer file")
	}
	want := fmt.Sprintf("%x", sha256.Sum256([]byte("new")))
	if inv.Files["/usr/lib/alias.so"].SHA256 != want {
		t.Fatal("hard link did not bind the new layer payload")
	}
	filename, s = ociFixture(t, old, layerBytes(t, layerEntry{name: "usr/bin/.wh.missing-tool"}))
	inv, err = ReadInventory(filename, s)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := inv.Files["/usr/bin/missing-tool"]; ok {
		t.Fatal("whiteout did not remove the file")
	}
}

func TestOCIInventoryRejectsForgeryAndUnsafeLayers(t *testing.T) {
	for _, kind := range []string{"manifest", "config", "owner", "architecture", "corrupt blob"} {
		t.Run(kind, func(t *testing.T) {
			filename, s := ociFixture(t, layerBytes(t, layerEntry{name: "file", body: "payload"}))
			switch kind {
			case "manifest":
				s.Source.Metadata.ManifestDigest = "sha256:" + strings.Repeat("0", 64)
			case "config":
				s.Source.Metadata.ImageID = "sha256:" + strings.Repeat("0", 64)
			case "owner":
				s.Source.Metadata.Labels["org.opencontainers.image.title"] = "wrong"
			case "architecture":
				s.Source.Metadata.Architecture = "arm64"
			case "corrupt blob":
				data, err := os.ReadFile(filename)
				if err != nil {
					t.Fatal(err)
				}
				data = bytes.Replace(data, []byte("payload"), []byte("changed"), 1)
				if err = os.WriteFile(filename, data, 0600); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := ReadInventory(filename, s); err == nil {
				t.Fatal("unbound archive accepted")
			}
		})
	}
	for name, entries := range map[string][]layerEntry{
		"parent whiteout":    {{name: "usr/bin/.wh..."}},
		"file below symlink": {{name: "usr", kind: tar.TypeSymlink, link: "other"}, {name: "usr/bin/tool", body: "payload"}},
		"traversal":          {{name: "../outside", body: "bad"}},
		"duplicate":          {{name: "file", body: "first"}, {name: "file", body: "second"}},
		"hardlink cycle":     {{name: "first", link: "second", kind: tar.TypeLink}, {name: "second", link: "first", kind: tar.TypeLink}},
		"invalid whiteout":   {{name: ".wh.file", body: "not empty"}},
	} {
		t.Run(name, func(t *testing.T) {
			filename, s := ociFixture(t, layerBytes(t, entries...))
			if _, err := ReadInventory(filename, s); err == nil {
				t.Fatal("unsafe layer accepted")
			}
		})
	}
}

func TestOCIInventoryGzipAndJSONCannotSupplyProof(t *testing.T) {
	concatenated := append(layerBytes(t, layerEntry{name: "first", body: "first"}), layerBytes(t, layerEntry{name: "hidden", body: "hidden"})...)
	filename, s := ociFixture(t, concatenated)
	if _, err := ReadInventory(filename, s); err == nil {
		t.Fatal("hidden concatenated tar accepted")
	}
	var compressed bytes.Buffer
	w := gzip.NewWriter(&compressed)
	_, _ = w.Write(layerBytes(t, layerEntry{name: "file", body: "payload"}))
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	filename, s = ociFixture(t, compressed.Bytes())
	if _, err := ReadInventory(filename, s); err != nil {
		t.Fatal(err)
	}
	bad := append([]byte(nil), compressed.Bytes()...)
	bad[len(bad)-1] ^= 1
	filename, s = ociFixture(t, bad)
	if _, err := ReadInventory(filename, s); err == nil {
		t.Fatal("bad gzip checksum accepted")
	}
	var forged SBOM
	if err := json.Unmarshal([]byte(`{"Inventory":{"ManifestDigest":"fake","Files":{"/bin/tool":{}}}}`), &forged); err != nil {
		t.Fatal(err)
	}
	if forged.Inventory != nil {
		t.Fatal("JSON injected trusted OCI inventory")
	}
}
