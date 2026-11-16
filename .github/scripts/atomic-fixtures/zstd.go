package main

import (
	"bytes"
	"compress/gzip"
	"encoding/binary"
	"encoding/json"
	"fmt"
)

// rawZstdFrame is a fixture-only Zstandard frame with one final raw block.
// Its <=128 KiB limit is intentional; it is not a general compressor. A raw
// block still requires Zstandard support and exercises Skopeo's index metadata.
// RFC 8878: single-segment frame, 8-byte content size, no dictionary/checksum.
func rawZstdFrame(data []byte) ([]byte, error) {
	if len(data) == 0 || len(data) > 128*1024 {
		return nil, fmt.Errorf("synthetic zstd block must be 1..128 KiB")
	}
	frame := []byte{0x28, 0xb5, 0x2f, 0xfd, 0xe0}
	frame = binary.LittleEndian.AppendUint64(frame, uint64(len(data)))
	header := uint32(len(data))<<3 | 1 // final block, raw type
	frame = append(frame, byte(header), byte(header>>8), byte(header>>16))
	return append(frame, data...), nil
}

// mixedArchive uses real tar layers in zstd and gzip frames, like the Atomic
// BuildKit export which exposed a preserve-digests failure in index publication.
// Native platform/source labels remain synthetic; no OS qualification is claimed.
func mixedArchive(arch, source, version string) ([]byte, error) {
	files, _, _, err := layout(arch, source, version)
	if err != nil {
		return nil, err
	}
	var index struct {
		Manifests []descriptor `json:"manifests"`
	}
	if err := json.Unmarshal(files["index.json"], &index); err != nil {
		return nil, err
	}
	original := index.Manifests[0]
	var manifest struct {
		Config descriptor   `json:"config"`
		Layers []descriptor `json:"layers"`
	}
	if err := json.Unmarshal(files["blobs/sha256/"+original.Digest[7:]], &manifest); err != nil {
		return nil, err
	}
	raw := files["blobs/sha256/"+manifest.Layers[0].Digest[7:]]
	zstd, err := rawZstdFrame(raw)
	if err != nil {
		return nil, err
	}
	gzipRaw, err := tarBytes(map[string][]byte{"gzip.txt": []byte(warning)})
	if err != nil {
		return nil, err
	}
	var gzipBytes bytes.Buffer
	writer := gzip.NewWriter(&gzipBytes)
	if _, err := writer.Write(gzipRaw); err != nil {
		return nil, err
	}
	if err := writer.Close(); err != nil {
		return nil, err
	}
	var config map[string]any
	if err := json.Unmarshal(files["blobs/sha256/"+manifest.Config.Digest[7:]], &config); err != nil {
		return nil, err
	}
	config["rootfs"] = map[string]any{"type": "layers", "diff_ids": []string{"sha256:" + sum(raw), "sha256:" + sum(gzipRaw)}}
	for _, d := range []descriptor{original, manifest.Config, manifest.Layers[0]} {
		delete(files, "blobs/sha256/"+d.Digest[7:])
	}
	add := func(media string, data []byte) descriptor {
		hash := sum(data)
		files["blobs/sha256/"+hash] = data
		return descriptor{Media: media, Digest: "sha256:" + hash, Size: len(data)}
	}
	layers := []descriptor{
		add("application/vnd.oci.image.layer.v1.tar+zstd", zstd),
		add("application/vnd.oci.image.layer.v1.tar+gzip", gzipBytes.Bytes()),
	}
	configDescriptor := add("application/vnd.oci.image.config.v1+json", jsonBytes(config))
	native := add("application/vnd.oci.image.manifest.v1+json", jsonBytes(map[string]any{
		"schemaVersion": 2, "mediaType": "application/vnd.oci.image.manifest.v1+json",
		"config": configDescriptor, "layers": layers,
	}))
	native.Annotations = map[string]string{"org.opencontainers.image.ref.name": "45"}
	files["index.json"] = jsonBytes(map[string]any{"schemaVersion": 2, "manifests": []descriptor{native}})
	return tarBytes(files)
}
