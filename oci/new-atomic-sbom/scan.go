package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

type sourceIdentity struct {
	Kind      string `json:"kind"`
	Archive   string `json:"archive,omitempty"`
	SHA256    string `json:"archive_sha256,omitempty"`
	Reference string `json:"reference,omitempty"`
	Digest    string `json:"requested_digest,omitempty"`
}
type sbomIdentity struct {
	Path     string `json:"path"`
	SHA256   string `json:"sha256"`
	Size     int64  `json:"size"`
	Format   string `json:"format"`
	Packages int    `json:"packages"`
}
type binding struct {
	SchemaVersion int            `json:"schema_version"`
	Operation     string         `json:"operation"`
	Source        sourceIdentity `json:"source"`
	Architecture  string         `json:"architecture"`
	OS            string         `json:"os"`
	Revision      string         `json:"source_revision,omitempty"`
	Native        descriptor     `json:"native_manifest"`
	Config        descriptor     `json:"config"`
	Layers        []descriptor   `json:"layers"`
	SBOM          sbomIdentity   `json:"sbom"`
	RPM           coverage       `json:"rpm_coverage"`
	Catalogers    []string       `json:"default_catalogers"`
	Scope         string         `json:"scope"`
	Exclusions    []string       `json:"exclusions"`
	Parallelism   int            `json:"parallelism"`
	GoMemoryLimit string         `json:"go_memory_limit"`
	SyftPeakRSS   int64          `json:"syft_peak_rss_bytes"`
	SyftRSSBudget int64          `json:"syft_rss_budget_bytes"`
}

func scan(ctx context.Context, workspace string, input options, password string, run runner) (b binding, o options, err error) {
	o, err = normalize(input)
	if err != nil {
		return b, o, err
	}
	if (password != "") != (o.Username != "") {
		return b, o, errors.New("registry user and password must be supplied together")
	}
	if password != "" && o.Reference == "" {
		return b, o, errors.New("credentials require a registry reference")
	}
	workspace, err = filepath.Abs(workspace)
	if err != nil {
		return b, o, err
	}
	workspace, err = filepath.EvalSymlinks(workspace)
	if err != nil {
		return b, o, err
	}
	if strings.ContainsAny(workspace, ":\x00\r\n") {
		return b, o, errors.New("unsupported workspace path")
	}
	root, err := os.OpenRoot(workspace)
	if err != nil {
		return b, o, err
	}
	defer root.Close()
	for _, path := range []string{o.Output, o.Binding} {
		if err = checkDestination(root, path); err != nil {
			return b, o, err
		}
	}
	b = binding{SchemaVersion: 1, Operation: "atomic-rootfs-sbom", Architecture: o.Architecture, OS: "linux",
		Catalogers: []string{"image"}, Scope: "squashed", Exclusions: []string{}, Parallelism: 1, GoMemoryLimit: "4GiB", SyftRSSBudget: syftRSSBudget}
	archivePath := ""
	if o.Archive != "" {
		info, e := root.Lstat(o.Archive)
		if e != nil {
			return b, o, e
		}
		if !info.Mode().IsRegular() {
			return b, o, errors.New("archive must be a regular file, not a symlink")
		}
		archivePath, err = filepath.EvalSymlinks(filepath.Join(workspace, o.Archive))
		if err != nil {
			return b, o, err
		}
		rel, e := filepath.Rel(workspace, archivePath)
		if e != nil || rel == ".." || strings.HasPrefix(rel, "../") {
			return b, o, errors.New("archive escapes workspace")
		}
		sum, _, e := hashFile(ctx, archivePath)
		if e != nil {
			return b, o, e
		}
		if sum != o.SHA256 {
			return b, o, errors.New("archive checksum mismatch")
		}
		b.Source = sourceIdentity{Kind: "archive", Archive: o.Archive, SHA256: sum}
	} else {
		b.Source = sourceIdentity{Kind: "registry", Reference: o.Reference, Digest: strings.Split(o.Reference, "@")[1]}
	}
	// Container-private, disk-backed scratch. Never put archives/rootfs/auth in
	// the caller's output tree or inherit a workspace TMPDIR.
	work, err := os.MkdirTemp("/tmp", "atomic-sbom-")
	if err != nil {
		return b, o, err
	}
	cleaned := false
	defer func() {
		if !cleaned {
			err = errors.Join(err, os.RemoveAll(work))
		}
	}()
	for _, dir := range []string{"home", "config"} {
		if err = os.Mkdir(filepath.Join(work, dir), 0700); err != nil {
			return b, o, err
		}
	}
	auth := filepath.Join(work, "auth.json")
	if err = os.WriteFile(auth, []byte(`{"auths":{}}`), 0600); err != nil {
		return b, o, err
	}
	call := func(name string, capture bool, args ...string) (commandResult, error) {
		return run(ctx, command{Name: name, Work: work, Capture: capture, Args: args})
	}
	inspect := func(source string) ([]byte, error) {
		r, e := call("skopeo", true, "--override-arch", o.Architecture, "--override-os", "linux", "inspect", "--raw", "--authfile", auth, source)
		return r.Output, e
	}
	source := "oci-archive:" + archivePath
	native := ""
	if o.Reference != "" {
		repository := strings.Split(o.Reference, "@")[0]
		registry := strings.Split(repository, "/")[0]
		if password != "" {
			if strings.ContainsAny(o.Username, ":\x00\r\n") {
				return b, o, errors.New("invalid registry username")
			}
			credentials, e := json.Marshal(map[string]any{"auths": map[string]any{registry: map[string]string{"auth": base64.StdEncoding.EncodeToString([]byte(o.Username + ":" + password))}}})
			if e != nil {
				return b, o, e
			}
			if err = os.WriteFile(auth, credentials, 0600); err != nil {
				return b, o, err
			}
		}
		raw, e := inspect("docker://" + o.Reference)
		if e != nil {
			return b, o, e
		}
		native, e = selectNative(raw, b.Source.Digest, o.Architecture)
		if e != nil {
			return b, o, e
		}
		source = "docker://" + repository + "@" + native
	}
	raw, err := inspect(source)
	if err != nil {
		return b, o, err
	}
	if native == "" {
		native = rawDigest(raw)
	}
	if rawDigest(raw) != native {
		return b, o, errors.New("selected native manifest changed")
	}
	var original manifest
	if err = json.Unmarshal(raw, &original); err != nil {
		return b, o, err
	}
	if original.SchemaVersion != 2 || original.MediaType != "application/vnd.oci.image.manifest.v1+json" {
		return b, o, errors.New("source must select a native OCI manifest")
	}
	layout := filepath.Join(work, "layout")
	if _, err = call("skopeo", false, "--override-arch", o.Architecture, "--override-os", "linux", "copy", "--preserve-digests", "--authfile", auth, source, "oci:"+layout+":scan"); err != nil {
		return b, o, err
	}
	if err = os.Remove(auth); err != nil {
		return b, o, err
	}
	b.Native, original, b.Revision, err = validateLayout(ctx, layout, native, o.Architecture, o.Revision)
	if err != nil {
		return b, o, err
	}
	b.Config, b.Layers = original.Config, original.Layers
	bundle := filepath.Join(work, "bundle")
	if _, err = call("umoci", false, "unpack", "--rootless", "--image", layout+":scan", bundle); err != nil {
		return b, o, err
	}
	// The source archive is untouched; only this verified temporary copy is
	// released. Syft needs the final rootfs, not another compressed layer store.
	if err = os.RemoveAll(layout); err != nil {
		return b, o, err
	}
	rootfs := filepath.Join(bundle, "rootfs")
	db, err := copyRPMDatabase(rootfs, work)
	if err != nil {
		return b, o, err
	}
	installed, inventorySum, err := queryRPM(ctx, work, run)
	if err != nil {
		return b, o, err
	}
	// Explicit config + clean environment preserves the full image cataloger
	// set for a directory source, including OSTree objects. No narrowing knobs.
	config := filepath.Join(work, "syft.yaml")
	policy, err := json.Marshal(map[string]any{"check-for-app-update": false, "default-catalogers": []string{"image"},
		"select-catalogers": []string{}, "exclude": []string{}, "scope": "squashed", "parallelism": 1,
		"source": map[string]string{"name": "verified-atomic-rootfs", "version": native, "base-path": rootfs}})
	if err != nil {
		return b, o, err
	}
	if err = os.WriteFile(config, policy, 0600); err != nil {
		return b, o, err
	}
	sbomPath := filepath.Join(work, "sbom.spdx.json")
	result, err := call("syft", false, "scan", "dir:"+rootfs, "--config", config, "--output", "spdx-json="+sbomPath)
	if err != nil {
		return b, o, err
	}
	b.SyftPeakRSS = result.PeakRSS
	if b.SyftPeakRSS <= 0 || b.SyftPeakRSS > syftRSSBudget {
		return b, o, errors.New("Syft process RSS is unavailable or exceeds the 7 GiB qualification budget")
	}
	b.RPM, b.SBOM.Packages, err = verifyCoverage(sbomPath, installed)
	if err != nil {
		return b, o, err
	}
	b.RPM.Database, b.RPM.InventorySHA256 = db, inventorySum
	b.SBOM.Path, b.SBOM.Format = o.Output, "spdx-json"
	b.SBOM.SHA256, b.SBOM.Size, err = hashFile(ctx, sbomPath)
	if err != nil {
		return b, o, err
	}
	if archivePath != "" {
		sum, _, e := hashFile(ctx, archivePath)
		if e != nil {
			return b, o, e
		}
		if sum != o.SHA256 {
			return b, o, errors.New("original archive changed during scanning")
		}
	}
	if err = ctx.Err(); err != nil {
		return b, o, err
	}
	encoded, err := json.MarshalIndent(b, "", "  ")
	if err != nil {
		return b, o, err
	}
	// Keep only an open read handle to the verified SPDX bytes. Cleanup must
	// succeed before any public output is created; unlinking the private file
	// does not alter its open descriptor on the supported Linux runtimes.
	ready, err := os.Open(sbomPath)
	if err != nil {
		return b, o, err
	}
	defer ready.Close()
	if err = os.RemoveAll(work); err != nil {
		return b, o, err
	}
	cleaned = true
	if err = publish(ctx, root, workspace, o, ready, b.SBOM.SHA256, append(encoded, '\n')); err != nil {
		return b, o, err
	}
	return b, o, nil
}
