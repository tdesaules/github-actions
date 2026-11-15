package main

import (
	"archive/tar"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"regexp"
	"strings"
)

type countedReader struct {
	ctx    context.Context
	reader io.Reader
	n      int64
}

func (r *countedReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	n, err := r.reader.Read(p)
	r.n += int64(n)
	return n, err
}

type archiveEntry struct{ offset, size int64 }

var descriptorDigestPattern = regexp.MustCompile(`^sha256:[a-f0-9]{64}$`)

type descriptor struct {
	MediaType string                             `json:"mediaType"`
	Digest    string                             `json:"digest"`
	Size      int64                              `json:"size"`
	Platform  *struct{ Architecture, OS string } `json:"platform,omitempty"`
}
type indexDocument struct {
	SchemaVersion int          `json:"schemaVersion"`
	Manifests     []descriptor `json:"manifests"`
}
type manifestDocument struct {
	SchemaVersion int          `json:"schemaVersion"`
	MediaType     string       `json:"mediaType"`
	Config        descriptor   `json:"config"`
	Layers        []descriptor `json:"layers"`
}

// One sequential pass hashes the exact tar and each blob without unpacking any
// layer. Metadata is read by recorded offsets, keeping memory and disk bounded.
func verifyArchive(ctx context.Context, f *os.File, arch string, labels map[string]string) (result, error) {
	info, err := f.Stat()
	if err != nil {
		return result{}, err
	}
	if !info.Mode().IsRegular() || info.Size() == 0 {
		return result{}, errors.New("empty or nonregular OCI archive")
	}
	h := sha256.New()
	counted := &countedReader{ctx: ctx, reader: io.TeeReader(f, h)}
	tr := tar.NewReader(counted)
	entries := map[string]archiveEntry{}
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return result{}, fmt.Errorf("invalid OCI tar: %w", err)
		}
		name := path.Clean(header.Name)
		if name == ".." || strings.HasPrefix(name, "../") || strings.HasPrefix(name, "/") {
			return result{}, errors.New("OCI tar path escapes archive")
		}
		if header.Typeflag == tar.TypeDir {
			continue
		}
		if header.Typeflag != tar.TypeReg || len(entries) >= 16384 {
			return result{}, errors.New("OCI archive contains unsupported entries")
		}
		if _, exists := entries[name]; exists {
			return result{}, errors.New("duplicate OCI tar entry")
		}
		entries[name] = archiveEntry{counted.n, header.Size}
		if strings.HasPrefix(name, "blobs/") {
			blob := sha256.New()
			if _, err = io.Copy(blob, tr); err != nil {
				return result{}, err
			}
			if name != "blobs/sha256/"+hex.EncodeToString(blob.Sum(nil)) {
				return result{}, errors.New("OCI blob content does not match its digest")
			}
		} else if _, err = io.Copy(io.Discard, tr); err != nil {
			return result{}, err
		}
	}
	if _, err = io.Copy(io.Discard, counted); err != nil {
		return result{}, err
	}
	after, err := f.Stat()
	if err != nil {
		return result{}, err
	}
	if counted.n != info.Size() || after.Size() != info.Size() || !after.ModTime().Equal(info.ModTime()) {
		return result{}, errors.New("OCI archive changed during verification")
	}
	read := func(name string, v any) error {
		e, ok := entries[name]
		if !ok || e.size <= 0 || e.size > 8<<20 {
			return fmt.Errorf("missing or oversized OCI metadata %s", name)
		}
		data := make([]byte, int(e.size))
		if _, err := f.ReadAt(data, e.offset); err != nil {
			return err
		}
		return json.Unmarshal(data, v)
	}
	blobPath := func(d descriptor) (string, error) {
		if !descriptorDigestPattern.MatchString(d.Digest) {
			return "", errors.New("OCI descriptor requires sha256 digest")
		}
		name := "blobs/sha256/" + strings.TrimPrefix(d.Digest, "sha256:")
		e, ok := entries[name]
		if !ok || e.size != d.Size {
			return "", errors.New("OCI descriptor blob missing or size mismatch")
		}
		return name, nil
	}
	var layout struct {
		Version string `json:"imageLayoutVersion"`
	}
	if err = read("oci-layout", &layout); err != nil || layout.Version != "1.0.0" {
		return result{}, errors.New("OCI image layout version 1.0.0 required")
	}
	var index indexDocument
	if err = read("index.json", &index); err != nil {
		return result{}, err
	}
	if index.SchemaVersion != 2 || len(index.Manifests) != 1 || index.Manifests[0].MediaType != "application/vnd.oci.image.manifest.v1+json" {
		return result{}, errors.New("export must contain exactly one native OCI image manifest, without attestations or nested indexes")
	}
	d := index.Manifests[0]
	if d.Platform != nil && (d.Platform.Architecture != arch || d.Platform.OS != "linux") {
		return result{}, errors.New("OCI index platform mismatch")
	}
	name, err := blobPath(d)
	if err != nil {
		return result{}, err
	}
	var manifest manifestDocument
	if err = read(name, &manifest); err != nil {
		return result{}, err
	}
	if manifest.SchemaVersion != 2 || manifest.MediaType != "application/vnd.oci.image.manifest.v1+json" || manifest.Config.MediaType != "application/vnd.oci.image.config.v1+json" {
		return result{}, errors.New("export must use OCI image manifest/config media types")
	}
	name, err = blobPath(manifest.Config)
	if err != nil {
		return result{}, err
	}
	var config struct {
		Architecture string `json:"architecture"`
		OS           string `json:"os"`
		Config       struct {
			Labels map[string]string `json:"Labels"`
		} `json:"config"`
	}
	if err = read(name, &config); err != nil {
		return result{}, err
	}
	if config.Architecture != arch || config.OS != "linux" {
		return result{}, errors.New("OCI image configuration is not the requested native platform")
	}
	for key, value := range labels {
		if got, ok := config.Config.Labels[key]; !ok || got != value {
			return result{}, fmt.Errorf("exported OCI label %s differs from requested label", key)
		}
	}
	for _, layer := range manifest.Layers {
		switch layer.MediaType {
		case "application/vnd.oci.image.layer.v1.tar", "application/vnd.oci.image.layer.v1.tar+gzip", "application/vnd.oci.image.layer.v1.tar+zstd":
		default:
			return result{}, errors.New("unsupported OCI layer media type")
		}
		if _, err = blobPath(layer); err != nil {
			return result{}, err
		}
	}
	return result{SHA256: hex.EncodeToString(h.Sum(nil)), Size: counted.n, ManifestDigest: d.Digest}, nil
}
