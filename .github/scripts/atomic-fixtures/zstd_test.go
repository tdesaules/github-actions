package main

import (
	"bytes"
	"compress/gzip"
	"encoding/binary"
	"encoding/json"
	"io"
	"strings"
	"testing"
)

func TestMixedCompressionArchivesBindRealTarLayers(t *testing.T) {
	for _, arch := range []string{"amd64", "arm64"} {
		archive, err := mixedArchive(arch, strings.Repeat("a", 40), "1.0.0")
		if err != nil {
			t.Fatal(err)
		}
		again, err := mixedArchive(arch, strings.Repeat("a", 40), "1.0.0")
		if err != nil || !bytes.Equal(archive, again) {
			t.Fatal("mixed-compression fixture is not deterministic", err)
		}
		files := untar(t, archive)
		var index struct {
			Manifests []descriptor `json:"manifests"`
		}
		if err := json.Unmarshal(files["index.json"], &index); err != nil {
			t.Fatal(err)
		}
		blob := func(d descriptor) []byte {
			t.Helper()
			b := files["blobs/sha256/"+strings.TrimPrefix(d.Digest, "sha256:")]
			if len(b) != d.Size || "sha256:"+sum(b) != d.Digest {
				t.Fatal("descriptor digest/size does not bind blob")
			}
			return b
		}
		var manifest struct {
			Config descriptor   `json:"config"`
			Layers []descriptor `json:"layers"`
		}
		if err := json.Unmarshal(blob(index.Manifests[0]), &manifest); err != nil {
			t.Fatal(err)
		}
		var config struct {
			Architecture string `json:"architecture"`
			RootFS       struct {
				DiffIDs []string `json:"diff_ids"`
			} `json:"rootfs"`
		}
		if err := json.Unmarshal(blob(manifest.Config), &config); err != nil {
			t.Fatal(err)
		}
		if config.Architecture != arch || len(manifest.Layers) != 2 ||
			manifest.Layers[0].Media != "application/vnd.oci.image.layer.v1.tar+zstd" ||
			manifest.Layers[1].Media != "application/vnd.oci.image.layer.v1.tar+gzip" {
			t.Fatal("wrong native platform or mixed layer media types")
		}
		zstd := blob(manifest.Layers[0])
		if len(zstd) < 16 || !bytes.Equal(zstd[:5], []byte{0x28, 0xb5, 0x2f, 0xfd, 0xe0}) {
			t.Fatal("not the bounded single-segment Zstandard fixture format")
		}
		contentSize := binary.LittleEndian.Uint64(zstd[5:13])
		block := uint32(zstd[13]) | uint32(zstd[14])<<8 | uint32(zstd[15])<<16
		if block&7 != 1 || uint64(block>>3) != contentSize || int(contentSize) != len(zstd)-16 {
			t.Fatal("invalid final raw block or frame content size")
		}
		raw := zstd[16:]
		if "sha256:"+sum(raw) != config.RootFS.DiffIDs[0] || !bytes.Contains(untar(t, raw)["hello.txt"], []byte(warning)) {
			t.Fatal("zstd diff ID or tar payload mismatch")
		}
		reader, err := gzip.NewReader(bytes.NewReader(blob(manifest.Layers[1])))
		if err != nil {
			t.Fatal(err)
		}
		raw, err = io.ReadAll(reader)
		reader.Close()
		if err != nil || "sha256:"+sum(raw) != config.RootFS.DiffIDs[1] ||
			!bytes.Equal(untar(t, raw)["gzip.txt"], []byte(warning)) {
			t.Fatal("gzip diff ID or tar payload mismatch", err)
		}
	}
}

func TestRawZstdFixtureRejectsUnboundedBlocks(t *testing.T) {
	for _, n := range []int{0, 128*1024 + 1} {
		if _, err := rawZstdFrame(make([]byte, n)); err == nil {
			t.Fatalf("accepted %d-byte raw block", n)
		}
	}
}
