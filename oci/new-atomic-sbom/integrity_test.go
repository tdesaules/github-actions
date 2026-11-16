package main

import (
	"archive/tar"
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestIndexIdentityAndAmbiguity(t *testing.T) {
	d := descriptor{MediaType: "application/vnd.oci.image.manifest.v1+json", Digest: "sha256:" + strings.Repeat("a", 64), Size: 100,
		Platform: &struct {
			Architecture string `json:"architecture"`
			OS           string `json:"os"`
		}{"amd64", "linux"}}
	for _, tc := range []struct {
		name string
		ds   []descriptor
		arch string
		ok   bool
	}{
		{"one", []descriptor{d}, "amd64", true}, {"wrong-arch", []descriptor{d}, "arm64", false},
		{"duplicate", []descriptor{d, d}, "amd64", false}, {"empty", nil, "amd64", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw := jsonBytes(manifest{SchemaVersion: 2, MediaType: "application/vnd.oci.image.index.v1+json", Manifests: tc.ds})
			_, err := selectNative(raw, rawDigest(raw), tc.arch)
			if (err == nil) != tc.ok {
				t.Fatal(err)
			}
			if _, err := selectNative(raw, "sha256:"+strings.Repeat("f", 64), tc.arch); err == nil {
				t.Fatal("wrong source digest accepted")
			}
		})
	}
}
func TestMissingAndMalformedCoverage(t *testing.T) {
	f := newFixture(t, "amd64")
	installed, _, err := inventory([]byte("niri|0|25.11|1.fc45|x86_64\n"))
	if err != nil {
		t.Fatal(err)
	}
	for _, data := range [][]byte{nil, []byte("{}"), append(f.sbom, []byte("{}")...), spdxFixture("aarch64", true)} {
		path := filepath.Join(t.TempDir(), "sbom.json")
		_ = os.WriteFile(path, data, 0600)
		if _, _, err := verifyCoverage(path, installed); err == nil {
			t.Fatal("invalid coverage accepted")
		}
	}
	if _, _, err := inventory(nil); err == nil {
		t.Fatal("empty RPM inventory accepted")
	}
}
func TestCancellationDoesNotPublishPartialEvidence(t *testing.T) {
	f := newFixture(t, "amd64")
	m := &mockTools{t: t, fixture: f}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	_, _, err := scan(ctx, f.root, f.options, "", func(ctx context.Context, c command) (commandResult, error) {
		if c.Name == "syft" {
			cancel()
			return commandResult{}, ctx.Err()
		}
		return m.run(ctx, c)
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	for _, path := range []string{f.options.Output, f.options.Output + ".binding.json"} {
		if _, err := os.Stat(filepath.Join(f.root, path)); !os.IsNotExist(err) {
			t.Fatal("partial success output survived")
		}
	}
	assertClean(t, m)
}
func TestOutputRacePreservesUnrelatedBinding(t *testing.T) {
	f := newFixture(t, "amd64")
	root, err := os.OpenRoot(f.root)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	o, err := normalize(f.options)
	if err != nil {
		t.Fatal(err)
	}
	_ = os.MkdirAll(filepath.Join(f.root, "reports"), 0755)
	_ = os.WriteFile(filepath.Join(f.root, o.Binding), []byte("concurrent writer"), 0644)
	if err := publish(context.Background(), root, f.root, o, bytes.NewReader(f.sbom), "", nil); err == nil {
		t.Fatal("output collision accepted")
	}
	if _, err := os.Stat(filepath.Join(f.root, o.Output)); !os.IsNotExist(err) {
		t.Fatal("owned partial SBOM survived")
	}
	got, _ := os.ReadFile(filepath.Join(f.root, o.Binding))
	if string(got) != "concurrent writer" {
		t.Fatal("unrelated binding overwritten")
	}
}

// This is a compression compatibility check, not the hosted performance fixture.
// The mandatory hosted positive scan separately uses the real 269-layer OS.
func TestRealZstdExtractionPreservesNativeDigest(t *testing.T) {
	if os.Getenv("ATOMIC_SBOM_TOOL_INTEGRATION") != "1" {
		t.Skip("tool integration is run inside the built Fedora action image")
	}
	for _, tool := range []string{"skopeo", "umoci"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Fatal(err)
		}
	}
	work := t.TempDir()
	for _, dir := range []string{"home", "config", "original/blobs/sha256"} {
		if err := os.MkdirAll(filepath.Join(work, dir), 0700); err != nil {
			t.Fatal(err)
		}
	}
	var tarData bytes.Buffer
	tw := tar.NewWriter(&tarData)
	body := []byte("native zstd retained\n")
	if err := tw.WriteHeader(&tar.Header{Name: "zstd-marker", Mode: 0644, Size: int64(len(body))}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write(body); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	// RFC 8878 single-segment frame with one final raw block (no compressor
	// dependency). Actual Skopeo and umoci must decode it; media labels alone
	// cannot make this test pass.
	raw := tarData.Bytes()
	frame := binary.LittleEndian.AppendUint64([]byte{0x28, 0xb5, 0x2f, 0xfd, 0xe0}, uint64(len(raw)))
	header := uint32(len(raw))<<3 | 1
	frame = append(frame, byte(header), byte(header>>8), byte(header>>16))
	frame = append(frame, raw...)
	add := func(data []byte, media string) descriptor {
		d := descriptor{MediaType: media, Digest: rawDigest(data), Size: int64(len(data))}
		if err := os.WriteFile(filepath.Join(work, "original/blobs/sha256", d.Digest[7:]), data, 0600); err != nil {
			t.Fatal(err)
		}
		return d
	}
	layer := add(frame, "application/vnd.oci.image.layer.v1.tar+zstd")
	cfg := add(jsonBytes(map[string]any{"architecture": "amd64", "os": "linux", "config": map[string]any{}, "rootfs": map[string]any{"type": "layers", "diff_ids": []string{rawDigest(raw)}}}), "application/vnd.oci.image.config.v1+json")
	m := add(jsonBytes(manifest{SchemaVersion: 2, MediaType: "application/vnd.oci.image.manifest.v1+json", Config: cfg, Layers: []descriptor{layer}}), "application/vnd.oci.image.manifest.v1+json")
	_ = os.WriteFile(filepath.Join(work, "original/oci-layout"), []byte(`{"imageLayoutVersion":"1.0.0"}`), 0600)
	_ = os.WriteFile(filepath.Join(work, "original/index.json"), jsonBytes(manifest{SchemaVersion: 2, Manifests: []descriptor{m}}), 0600)
	ctx := context.Background()
	_, err := execute(ctx, command{Name: "skopeo", Work: work, Args: []string{"copy", "--preserve-digests", "oci:" + filepath.Join(work, "original"), "oci:" + filepath.Join(work, "copied") + ":scan"}})
	if err != nil {
		t.Fatal(err)
	}
	_, got, _, err := validateLayout(ctx, filepath.Join(work, "copied"), m.Digest, "amd64", "")
	if err != nil {
		t.Fatal(err)
	}
	if got.Layers[0].Digest != layer.Digest || got.Layers[0].MediaType != layer.MediaType {
		t.Fatal("zstd was recompressed or changed")
	}
	if _, err = execute(ctx, command{Name: "umoci", Work: work, Args: []string{"unpack", "--rootless", "--image", filepath.Join(work, "copied") + ":scan", filepath.Join(work, "bundle")}}); err != nil {
		t.Fatal(err)
	}
	gotBytes, err := os.ReadFile(filepath.Join(work, "bundle/rootfs/zstd-marker"))
	if err != nil || !bytes.Equal(gotBytes, body) {
		t.Fatal("zstd filesystem content mismatch")
	}
}

func TestHostedRealFixtureBinding(t *testing.T) {
	path := os.Getenv("ATOMIC_SBOM_FIXTURE_BINDING")
	if path == "" {
		t.Skip("real native fixture binding supplied by the dedicated hosted qualification step")
	}
	root := os.Getenv("GITHUB_WORKSPACE")
	if root == "" {
		t.Fatal("hosted workspace required")
	}
	data, err := os.ReadFile(filepath.Join(root, path))
	if err != nil {
		t.Fatal(err)
	}
	var b binding
	if err = json.Unmarshal(data, &b); err != nil {
		t.Fatal(err)
	}
	if b.SchemaVersion != 1 || b.Operation != "atomic-rootfs-sbom" || b.Source.Kind != "archive" ||
		b.Source.SHA256 != os.Getenv("ATOMIC_SBOM_FIXTURE_SHA256") ||
		b.Source.Archive != os.Getenv("ATOMIC_SBOM_FIXTURE_ARCHIVE") ||
		b.Architecture != os.Getenv("ATOMIC_SBOM_FIXTURE_ARCH") ||
		b.Revision != os.Getenv("ATOMIC_SBOM_FIXTURE_REVISION") {
		t.Fatal("fixture/source/architecture binding mismatch")
	}
	if len(b.Layers) < 200 || b.RPM.Installed < 1000 || b.RPM.Covered != b.RPM.Installed || b.SBOM.Packages < b.RPM.Installed ||
		b.SyftPeakRSS <= 0 || b.SyftPeakRSS > syftRSSBudget {
		t.Fatal("real OS coverage or RSS budget qualification missing")
	}
	sum, _, err := hashFile(context.Background(), filepath.Join(root, b.Source.Archive))
	if err != nil || sum != b.Source.SHA256 {
		t.Fatal("original fixture archive changed")
	}
	sum, size, err := hashFile(context.Background(), filepath.Join(root, b.SBOM.Path))
	if err != nil || sum != b.SBOM.SHA256 || size != b.SBOM.Size {
		t.Fatal("SBOM binding does not match retained output")
	}
	if !digestPattern.MatchString(b.Native.Digest) || !digestPattern.MatchString(b.Config.Digest) {
		t.Fatal("missing immutable native identity")
	}
}
