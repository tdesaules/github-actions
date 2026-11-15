package atomicrelease

import (
	"crypto/sha256"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func fixture(t *testing.T) (string, Bundle, []Payload) {
	t.Helper()
	dir := t.TempDir()
	source := strings.Repeat("a", 40)
	makeReceipt := func(arch, component, name string) Receipt {
		path := filepath.Join(dir, arch, name)
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		data := []byte(arch + component)
		if err := os.WriteFile(path, data, 0644); err != nil {
			t.Fatal(err)
		}
		sum, _, err := checksum(path)
		if err != nil {
			t.Fatal(err)
		}
		return Receipt{SchemaVersion: 1, Component: component, Arch: arch, SourceSHA: source, Version: "1.2.3",
			Archive: name, SHA256: sum, Size: int64(len(data)), ManifestDigest: "sha256:" + strings.Repeat("b", 64),
			Tested: true, Qualification: "userspace", RunID: "42", RunAttempt: "1", Checks: completeChecks(), Packages: []string{"niri"}}
	}
	b := Bundle{AMD64: makeReceipt("amd64", "os", "image.oci.tar"), ARM64: makeReceipt("arm64", "os", "image.oci.tar"), ISO: makeReceipt("amd64", "iso", "installer.iso")}
	b.ISO.Qualification = "installer-media"
	b.ISO.OSArchiveSHA256 = b.AMD64.SHA256
	b.ISO.BuildRunID = "42"
	b.ISO.BuildRunAttempt = "1"
	for path, r := range map[string]Receipt{"amd64/qualification.json": b.AMD64, "arm64/qualification.json": b.ARM64, "amd64/iso-qualification.json": b.ISO} {
		if err := Write(filepath.Join(dir, path), r); err != nil {
			t.Fatal(err)
		}
	}
	build := b.ISO
	build.Tested = false
	if err := Write(filepath.Join(dir, "amd64", "installer.iso.build.json"), build); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "amd64", "installer.iso-CHECKSUM"), []byte(b.ISO.SHA256+"  installer.iso\n"), 0644); err != nil {
		t.Fatal(err)
	}
	files := []Payload{}
	for name, typ := range map[string]string{"installer.iso": "application/x-iso9660-image", "installer.iso-CHECKSUM": "text/plain", "installer.iso.build.json": "application/json"} {
		sum, _, err := checksum(filepath.Join(dir, "amd64", name))
		if err != nil {
			t.Fatal(err)
		}
		files = append(files, Payload{name, sum, typ})
	}
	b.ISO.Files = files
	if err := Write(filepath.Join(dir, "amd64", "iso-qualification.json"), b.ISO); err != nil {
		t.Fatal(err)
	}
	return dir, b, files
}
func TestVerifyBundle(t *testing.T) {
	for _, tc := range []struct {
		name         string
		mutate       func(string, *Bundle)
		run, attempt string
		want         bool
	}{
		{"valid", nil, "42", "1", true},
		{"partial-retry", nil, "42", "2", true},
		{"wrong-run", nil, "43", "1", false},
		{"future-attempt", nil, "42", "0", false},
		{"missing-arm64", func(d string, b *Bundle) { _ = os.Remove(filepath.Join(d, "arm64", "qualification.json")) }, "42", "1", false},
		{"tampered-os", func(d string, b *Bundle) {
			_ = os.WriteFile(filepath.Join(d, "arm64", b.ARM64.Archive), []byte("wrong"), 0644)
		}, "42", "1", false},
		{"tampered-iso", func(d string, b *Bundle) {
			_ = os.WriteFile(filepath.Join(d, "amd64", b.ISO.Archive), []byte("wrong"), 0644)
		}, "42", "1", false},
		{"wrong-embedded-os", func(d string, b *Bundle) {
			b.ISO.OSArchiveSHA256 = strings.Repeat("c", 64)
			_ = Write(filepath.Join(d, "amd64", "iso-qualification.json"), b.ISO)
		}, "42", "1", false},
		{"wrong-manifest", func(d string, b *Bundle) {
			b.ISO.ManifestDigest = "sha256:" + strings.Repeat("c", 64)
			_ = Write(filepath.Join(d, "amd64", "iso-qualification.json"), b.ISO)
		}, "42", "1", false},
		{"wrong-version", func(d string, b *Bundle) {
			b.ARM64.Version = "1.2.4"
			_ = Write(filepath.Join(d, "arm64", "qualification.json"), b.ARM64)
		}, "42", "1", false},
		{"wrong-source", func(d string, b *Bundle) {
			b.ARM64.SourceSHA = strings.Repeat("c", 40)
			_ = Write(filepath.Join(d, "arm64", "qualification.json"), b.ARM64)
		}, "42", "1", false},
		{"false-boot-claim", func(d string, b *Bundle) {
			b.ISO.BootTested = true
			_ = Write(filepath.Join(d, "amd64", "iso-qualification.json"), b.ISO)
		}, "42", "1", false},
		{"path-traversal", func(d string, b *Bundle) {
			b.ARM64.Archive = "../image.oci.tar"
			_ = Write(filepath.Join(d, "arm64", "qualification.json"), b.ARM64)
		}, "42", "1", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir, b, _ := fixture(t)
			if tc.mutate != nil {
				tc.mutate(dir, &b)
			}
			_, err := Verify(dir, b.AMD64.SourceSHA, "1.2.3", tc.run, tc.attempt)
			if (err == nil) != tc.want {
				t.Fatalf("got %v", err)
			}
		})
	}
}
func TestTagReexportUsesOriginalISOOSBinding(t *testing.T) {
	dir, b, files := fixture(t)
	original := b.AMD64.SHA256
	path := filepath.Join(dir, "amd64", b.AMD64.Archive)
	if err := os.WriteFile(path, []byte("same manifest, different transport tar"), 0644); err != nil {
		t.Fatal(err)
	}
	b.AMD64.SHA256, _, _ = checksum(path)
	if err := Write(filepath.Join(dir, "amd64", "qualification.json"), b.AMD64); err != nil {
		t.Fatal(err)
	}
	if _, err := Verify(dir, b.AMD64.SourceSHA, "1.2.3", "42", "1"); err == nil {
		t.Fatal("preparation accepted different ISO OS archive")
	}
	if _, err := verify(dir, b.AMD64.SourceSHA, "1.2.3", "42", "1", original); err != nil {
		t.Fatal(err)
	}
	if err := VerifyPayloads(dir, files, b.ISO); err != nil {
		t.Fatal(err)
	}
	files[0].SHA256 = strings.Repeat("f", 64)
	if err := VerifyPayloads(dir, files, b.ISO); err == nil {
		t.Fatal("tampered payload list accepted")
	}
}

func TestGateDerivesPersistedFiles(t *testing.T) {
	dir, b, _ := fixture(t)
	t.Setenv("GITHUB_RUN_ID", "42")
	t.Setenv("GITHUB_RUN_ATTEMPT", "2")
	output := filepath.Join(t.TempDir(), "outputs")
	if err := os.WriteFile(output, nil, 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GITHUB_OUTPUT", output)
	o := Options{Directory: dir, Source: b.AMD64.SourceSHA, Version: b.AMD64.Version, Output: filepath.Join(t.TempDir(), "bundle.json")}
	if err := VerifyAction(o); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "iso_files=") || !strings.Contains(string(data), "installer.iso.build.json") {
		t.Fatal("persisted payload list not exported")
	}
	b.ISO.Files = nil
	if err = Write(filepath.Join(dir, "amd64", "iso-qualification.json"), b.ISO); err != nil {
		t.Fatal(err)
	}
	if err = VerifyAction(o); err == nil {
		t.Fatal("missing persisted payload list accepted")
	}
}
func evidenceFixture(t *testing.T) Evidence {
	_, b, files := fixture(t)
	return Evidence{SchemaVersion: 1, Version: "1.2.3", SourceSHA: b.AMD64.SourceSHA,
		Image: "ghcr.io/owner/os@sha256:" + strings.Repeat("d", 64), ISO: "ghcr.io/owner/os-iso@sha256:" + strings.Repeat("e", 64),
		ISOFile: b.ISO.Archive, ISOSHA256: b.ISO.SHA256, Architectures: []string{"amd64", "arm64"}, Qualified: b, ISOFiles: files}
}
func TestEvidenceAndManifestBinding(t *testing.T) {
	e := evidenceFixture(t)
	if err := ValidateEvidence(e, e.Version, "owner/os"); err != nil {
		t.Fatal(err)
	}
	if err := ValidateEvidence(e, e.Version, "other/os"); err == nil {
		t.Fatal("wrong repository accepted")
	}
	bad := e
	bad.Image = "ghcr.io/owner/os:latest"
	if err := ValidateEvidence(bad, e.Version, "owner/os"); err == nil {
		t.Fatal("mutable reference accepted")
	}
	bad = e
	bad.ISOFiles = bad.ISOFiles[:2]
	if err := ValidateEvidence(bad, e.Version, "owner/os"); err == nil {
		t.Fatal("incomplete ISO set accepted")
	}
	m := manifest{MediaType: "application/vnd.oci.image.index.v1+json"}
	for _, r := range []Receipt{e.Qualified.AMD64, e.Qualified.ARM64} {
		d := descriptor{Digest: r.ManifestDigest}
		d.Platform.Architecture = r.Arch
		d.Platform.OS = "linux"
		m.Manifests = append(m.Manifests, d)
	}
	raw, _ := json.Marshal(m)
	if err := verifyIndex(raw, e.Qualified); err != nil {
		t.Fatal(err)
	}
	m.Manifests[1].Platform.Architecture = "amd64"
	raw, _ = json.Marshal(m)
	if err := verifyIndex(raw, e.Qualified); err == nil {
		t.Fatal("duplicate architecture accepted")
	}
	iso := manifest{ArtifactType: "application/vnd.niri-tonic.installer.v1"}
	for _, f := range e.ISOFiles {
		d := descriptor{MediaType: f.MediaType, Digest: "sha256:" + f.SHA256, Annotations: map[string]string{"org.opencontainers.image.title": f.Path}}
		if f.Path == e.ISOFile {
			d.Size = e.Qualified.ISO.Size
		}
		iso.Layers = append(iso.Layers, d)
	}
	raw, _ = json.Marshal(iso)
	if err := verifyISO(raw, e.Qualified.ISO, e.ISOFiles); err != nil {
		t.Fatal(err)
	}
	iso.Layers[0].Digest = fmtDigest([]byte("wrong"))
	raw, _ = json.Marshal(iso)
	if err := verifyISO(raw, e.Qualified.ISO, e.ISOFiles); err == nil {
		t.Fatal("wrong published payload accepted")
	}
}
func fmtDigest(b []byte) string {
	s := sha256.Sum256(b)
	const hex = "0123456789abcdef"
	out := "sha256:"
	for _, c := range s {
		out += string([]byte{hex[c>>4], hex[c&15]})
	}
	return out
}
