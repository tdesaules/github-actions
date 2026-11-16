package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var digestPattern = regexp.MustCompile(`^sha256:[a-f0-9]{64}$`)
var checksumPattern = regexp.MustCompile(`^[a-f0-9]{64}$`)
var revisionPattern = regexp.MustCompile(`^[a-f0-9]{40}$`)
var referencePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9.-]*(?::[0-9]+)?/[a-z0-9][a-z0-9._/-]*@sha256:[a-f0-9]{64}$`)

type descriptor struct {
	MediaType string `json:"mediaType"`
	Digest    string `json:"digest"`
	Size      int64  `json:"size"`
	Platform  *struct {
		Architecture string `json:"architecture"`
		OS           string `json:"os"`
	} `json:"platform,omitempty"`
}
type manifest struct {
	SchemaVersion int          `json:"schemaVersion"`
	MediaType     string       `json:"mediaType"`
	Manifests     []descriptor `json:"manifests,omitempty"`
	Config        descriptor   `json:"config"`
	Layers        []descriptor `json:"layers"`
}
type contextReader struct {
	ctx context.Context
	io.Reader
}

func (r contextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.Reader.Read(p)
}
func hashFile(ctx context.Context, path string) (string, int64, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return "", 0, err
	}
	if !info.Mode().IsRegular() {
		return "", 0, errors.New("expected a regular file")
	}
	f, err := os.Open(path)
	if err != nil {
		return "", 0, err
	}
	defer f.Close()
	h := sha256.New()
	n, err := io.Copy(h, contextReader{ctx, f})
	return hex.EncodeToString(h.Sum(nil)), n, err
}
func rawDigest(raw []byte) string { return fmt.Sprintf("sha256:%x", sha256.Sum256(raw)) }

func relative(path string) error {
	if path == "" || filepath.IsAbs(path) || strings.ContainsAny(path, ":\\\x00\r\n") {
		return errors.New("paths must be workspace-relative without transport separators or control characters")
	}
	for _, part := range strings.Split(path, "/") {
		if part == ".." {
			return errors.New("parent traversal is forbidden")
		}
	}
	if filepath.Clean(path) == "." {
		return errors.New("a file path is required")
	}
	return nil
}
func normalize(o options) (options, error) {
	if (o.Archive == "") == (o.Reference == "") {
		return o, errors.New("exactly one archive or immutable registry reference is required")
	}
	if o.Architecture != "amd64" && o.Architecture != "arm64" {
		return o, errors.New("architecture must be amd64 or arm64")
	}
	if o.Revision != "" && !revisionPattern.MatchString(o.Revision) {
		return o, errors.New("revision must be a full source SHA")
	}
	if o.Archive != "" {
		if err := relative(o.Archive); err != nil {
			return o, err
		}
		o.Archive = filepath.Clean(o.Archive)
		if !checksumPattern.MatchString(o.SHA256) {
			return o, errors.New("archive SHA-256 is required")
		}
		if o.Username != "" {
			return o, errors.New("registry credentials are only accepted for registry sources")
		}
	} else if !referencePattern.MatchString(o.Reference) || o.SHA256 != "" {
		return o, errors.New("reference must be registry/repository@sha256:digest; archive checksum is not a registry input")
	}
	if o.Reference != "" {
		for _, part := range strings.Split(strings.Split(o.Reference, "@")[0], "/") {
			if part == "" || part == "." || part == ".." {
				return o, errors.New("registry repository must be canonical")
			}
		}
	}
	if o.Binding == "" {
		o.Binding = o.Output + ".binding.json"
	}
	for _, p := range []string{o.Output, o.Binding} {
		if err := relative(p); err != nil {
			return o, err
		}
	}
	o.Output = filepath.Clean(o.Output)
	o.Binding = filepath.Clean(o.Binding)
	if o.Output == o.Binding || o.Output == o.Archive || o.Binding == o.Archive {
		return o, errors.New("SBOM, binding and input archive must have distinct paths")
	}
	if strings.HasPrefix(o.Output, o.Binding+"/") || strings.HasPrefix(o.Binding, o.Output+"/") {
		return o, errors.New("output files cannot be ancestors of one another")
	}
	return o, nil
}

// A registry reference may name an index, but its selected child must be an
// unambiguous native OCI manifest. No schema conversion is permitted.
func selectNative(raw []byte, expected, arch string) (string, error) {
	if rawDigest(raw) != expected {
		return "", errors.New("registry response does not match requested digest")
	}
	var m manifest
	if err := json.Unmarshal(raw, &m); err != nil {
		return "", err
	}
	if m.SchemaVersion != 2 {
		return "", errors.New("manifest schema version 2 required")
	}
	if m.MediaType == "application/vnd.oci.image.manifest.v1+json" {
		return expected, nil
	}
	if m.MediaType != "application/vnd.oci.image.index.v1+json" && m.MediaType != "application/vnd.docker.distribution.manifest.list.v2+json" {
		return "", errors.New("OCI manifest or multi-platform index required")
	}
	native := ""
	for _, d := range m.Manifests {
		if d.Platform == nil || d.Platform.OS != "linux" || d.Platform.Architecture != arch {
			continue
		}
		if native != "" || d.MediaType != "application/vnd.oci.image.manifest.v1+json" || !digestPattern.MatchString(d.Digest) || d.Size <= 0 {
			return "", errors.New("ambiguous or invalid requested native OCI descriptor")
		}
		native = d.Digest
	}
	if native == "" {
		return "", errors.New("requested Linux architecture absent from index")
	}
	return native, nil
}

func validateLayout(ctx context.Context, path, native, arch, revision string) (descriptor, manifest, string, error) {
	var index manifest
	data, err := os.ReadFile(filepath.Join(path, "index.json"))
	if err != nil {
		return descriptor{}, manifest{}, "", err
	}
	if err = json.Unmarshal(data, &index); err != nil {
		return descriptor{}, manifest{}, "", err
	}
	if index.SchemaVersion != 2 || len(index.Manifests) != 1 || index.Manifests[0].Digest != native ||
		index.Manifests[0].MediaType != "application/vnd.oci.image.manifest.v1+json" {
		return descriptor{}, manifest{}, "", errors.New("copy must preserve exactly the selected native OCI manifest")
	}
	d := index.Manifests[0]
	if d.Platform != nil && (d.Platform.Architecture != arch || d.Platform.OS != "linux") {
		return d, manifest{}, "", errors.New("native descriptor platform mismatch")
	}
	blob := func(d descriptor, metadata bool) ([]byte, error) {
		if !digestPattern.MatchString(d.Digest) || d.Size <= 0 {
			return nil, errors.New("invalid blob descriptor")
		}
		name := filepath.Join(path, "blobs", "sha256", d.Digest[7:])
		sum, size, err := hashFile(ctx, name)
		if err != nil {
			return nil, err
		}
		if "sha256:"+sum != d.Digest || size != d.Size {
			return nil, errors.New("blob hash or size mismatch")
		}
		if !metadata {
			return nil, nil
		}
		if size > 8<<20 {
			return nil, errors.New("oversized OCI metadata")
		}
		return os.ReadFile(name)
	}
	raw, err := blob(d, true)
	if err != nil {
		return d, manifest{}, "", err
	}
	var m manifest
	if err = json.Unmarshal(raw, &m); err != nil {
		return d, m, "", err
	}
	if m.SchemaVersion != 2 || m.MediaType != "application/vnd.oci.image.manifest.v1+json" || m.Config.MediaType != "application/vnd.oci.image.config.v1+json" {
		return d, m, "", errors.New("native OCI image/config media types required")
	}
	raw, err = blob(m.Config, true)
	if err != nil {
		return d, m, "", err
	}
	var config struct {
		Architecture string `json:"architecture"`
		OS           string `json:"os"`
		Config       struct {
			Labels map[string]string `json:"Labels"`
		} `json:"config"`
	}
	if err = json.Unmarshal(raw, &config); err != nil {
		return d, m, "", err
	}
	if config.Architecture != arch || config.OS != "linux" {
		return d, m, "", errors.New("native source architecture/OS mismatch")
	}
	gotRevision := config.Config.Labels["org.opencontainers.image.revision"]
	if revision != "" && gotRevision != revision {
		return d, m, "", errors.New("source revision mismatch")
	}
	for _, layer := range m.Layers {
		switch layer.MediaType {
		case "application/vnd.oci.image.layer.v1.tar", "application/vnd.oci.image.layer.v1.tar+gzip", "application/vnd.oci.image.layer.v1.tar+zstd":
		default:
			return d, m, "", errors.New("unsupported OCI filesystem layer")
		}
		if _, err = blob(layer, false); err != nil {
			return d, m, "", err
		}
	}
	// Never copy arbitrary image configuration, environment or labels into the
	// binding. Only a syntactically valid public commit identity may be included.
	if !revisionPattern.MatchString(gotRevision) {
		gotRevision = ""
	}
	return d, m, gotRevision, nil
}
