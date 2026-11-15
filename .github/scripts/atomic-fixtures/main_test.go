package main

import (
	"archive/tar"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func untar(t *testing.T, b []byte) map[string][]byte {
	t.Helper()
	r := tar.NewReader(bytes.NewReader(b))
	result := map[string][]byte{}
	for {
		h, err := r.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		if h.Uid != 0 || h.Gid != 0 || !h.ModTime.Equal(epoch) {
			t.Fatal("nondeterministic tar metadata")
		}
		result[h.Name], err = io.ReadAll(r)
		if err != nil {
			t.Fatal(err)
		}
	}
	return result
}

func TestOCIContentAndDeterminism(t *testing.T) {
	manifests := map[string]string{}
	for _, arch := range []string{"amd64", "arm64"} {
		files, archive, manifest, err := layout(arch, strings.Repeat("a", 40), "1.0.0")
		if err != nil {
			t.Fatal(err)
		}
		_, again, _, err := layout(arch, strings.Repeat("a", 40), "1.0.0")
		if err != nil || !bytes.Equal(archive, again) {
			t.Fatal("archive not deterministic")
		}
		extracted := untar(t, archive)
		if len(extracted) != len(files) {
			t.Fatal("archive does not contain entire layout")
		}
		for name, b := range files {
			if !bytes.Equal(b, extracted[name]) {
				t.Fatal(name)
			}
		}
		var index struct {
			Manifests []descriptor `json:"manifests"`
		}
		if err := json.Unmarshal(files["index.json"], &index); err != nil {
			t.Fatal(err)
		}
		if len(index.Manifests) != 1 || index.Manifests[0].Digest != manifest || index.Manifests[0].Annotations["org.opencontainers.image.ref.name"] != "45" {
			t.Fatal("wrong OCI reference")
		}
		var m struct {
			Config descriptor   `json:"config"`
			Layers []descriptor `json:"layers"`
		}
		if err := json.Unmarshal(files["blobs/sha256/"+strings.TrimPrefix(manifest, "sha256:")], &m); err != nil {
			t.Fatal(err)
		}
		for _, d := range append(m.Layers, m.Config) {
			b := files["blobs/sha256/"+strings.TrimPrefix(d.Digest, "sha256:")]
			if len(b) != d.Size || "sha256:"+sum(b) != d.Digest {
				t.Fatal("invalid descriptor binding")
			}
		}
		var config struct {
			Architecture string `json:"architecture"`
			Config       struct {
				Labels map[string]string `json:"Labels"`
			} `json:"config"`
			RootFS struct {
				DiffIDs []string `json:"diff_ids"`
			} `json:"rootfs"`
		}
		if err := json.Unmarshal(files["blobs/sha256/"+strings.TrimPrefix(m.Config.Digest, "sha256:")], &config); err != nil {
			t.Fatal(err)
		}
		if config.Architecture != arch || config.Config.Labels["org.opencontainers.image.revision"] != strings.Repeat("a", 40) || config.RootFS.DiffIDs[0] != m.Layers[0].Digest {
			t.Fatal("wrong source/arch/diff ID")
		}
		layer := untar(t, files["blobs/sha256/"+strings.TrimPrefix(m.Layers[0].Digest, "sha256:")])
		if !bytes.Contains(layer["hello.txt"], []byte(warning)) {
			t.Fatal("missing synthetic label")
		}
		manifests[arch] = manifest
	}
	if manifests["amd64"] == manifests["arm64"] {
		t.Fatal("native manifests must differ")
	}
}

func readJSON(t *testing.T, path string) map[string]any {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var v map[string]any
	if err = json.Unmarshal(b, &v); err != nil {
		t.Fatal(err)
	}
	return v
}

func TestBundleRelationshipsAndSpaces(t *testing.T) {
	t.Chdir(t.TempDir())
	o := options{"fixtures with spaces", strings.Repeat("b", 40), "2.3.4", "1234", "2"}
	out, err := generate(context.Background(), o, func(_ context.Context, tree, output string) error {
		if _, err := os.Stat(filepath.Join(tree, "niri-tomic-os/index.json")); err != nil {
			return err
		}
		return write(output, []byte("unit-test media placeholder; not an ISO\n"))
	})
	if err != nil {
		t.Fatal(err)
	}
	build := readJSON(t, out["build-receipt"])
	if build["tested"] != false || build["synthetic"] != true || build["run_id"] != "1234" {
		t.Fatal("incorrect raw receipt")
	}
	for _, arch := range []string{"amd64", "arm64"} {
		dir := filepath.Join(out["qualified-directory"], arch)
		r := readJSON(t, filepath.Join(dir, "qualification.json"))
		data, err := os.ReadFile(filepath.Join(dir, "image.oci.tar"))
		if err != nil {
			t.Fatal(err)
		}
		if r["sha256"] != sum(data) || r["tested"] != true || r["boot_tested"] != false || r["synthetic"] != true || r["source_sha"] != o.Source {
			t.Fatal("invalid synthetic OS receipt")
		}
		if len(r["checks"].(map[string]any)) != 8 || len(r["packages"].([]any)) == 0 {
			t.Fatal("missing protocol fixture fields")
		}
		if arch == "amd64" && out["os-archive-sha256"] != sum(data) {
			t.Fatal("raw/qualified archive mismatch")
		}
	}
	isoReport := readJSON(t, filepath.Join(out["qualified-directory"], "amd64", "iso-qualification.json"))
	if isoReport["run_id"] != "1234" || isoReport["build_run_id"] != "1234" || isoReport["build_run_attempt"] != "2" ||
		isoReport["os_archive_sha256"] != out["os-archive-sha256"] || isoReport["manifest_digest"] != out["os-manifest-digest"] {
		t.Fatal("incorrect ISO binding")
	}
	var payloads []payload
	if err := json.Unmarshal([]byte(out["iso-files"]), &payloads); err != nil {
		t.Fatal(err)
	}
	if len(payloads) != 3 {
		t.Fatal("wrong payload set")
	}
	for _, p := range payloads {
		raw, err := os.ReadFile(filepath.Join(o.Directory, "raw", p.Path))
		if err != nil {
			t.Fatal(err)
		}
		qualified, err := os.ReadFile(filepath.Join(out["qualified-directory"], "amd64", p.Path))
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(raw, qualified) || sum(raw) != p.SHA {
			t.Fatal("payload transfer identity lost")
		}
	}
	if payloads[2].Path != isoName+".build.json" {
		t.Fatal("original receipt name changed")
	}
	var good, bad []payload
	json.Unmarshal([]byte(out["transfer-files"]), &good)
	json.Unmarshal([]byte(out["tampered-transfer-files"]), &bad)
	data, _ := os.ReadFile(filepath.Join(out["transfer-directory"], "payload.txt"))
	if good[0].SHA != sum(data) || bad[0].SHA == sum(data) || bad[0].Path != good[0].Path {
		t.Fatal("invalid ORAS negative fixture")
	}
	if _, err = generate(context.Background(), o, media); err == nil {
		t.Fatal("overwriting output accepted")
	}
}

func TestValidationAndIsolation(t *testing.T) {
	base := options{Directory: "new fixtures", Source: strings.Repeat("a", 40), Version: "1.0.0"}
	for _, dir := range []string{"", ".", "../escape", "a/../../escape", "/absolute", "a\nb", `a\b`} {
		o := base
		o.Directory = dir
		if validate(&o, func(string) string { return "" }) == nil {
			t.Fatalf("accepted %q", dir)
		}
	}
	for _, env := range []map[string]string{
		{"GITHUB_ACTIONS": "true"},
		{"GITHUB_RUN_ID": "7"},
		{"GITHUB_RUN_ID": "7", "GITHUB_RUN_ATTEMPT": "0"},
	} {
		o := base
		if validate(&o, func(k string) string { return env[k] }) == nil {
			t.Fatal("invalid hosted identity accepted")
		}
	}
	o := base
	if err := validate(&o, func(string) string { return "" }); err != nil || o.Run != "1" || o.Attempt != "1" {
		t.Fatal("local defaults", err)
	}
	t.Chdir(t.TempDir())
	if err := os.Mkdir("real", 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("real", "link"); err != nil {
		t.Fatal(err)
	}
	o.Directory = "link/out"
	if _, err := generate(context.Background(), o, media); err == nil {
		t.Fatal("symlink ancestor accepted")
	}
	o.Directory = "failure"
	if _, err := generate(context.Background(), o, func(context.Context, string, string) error { return io.ErrUnexpectedEOF }); err == nil {
		t.Fatal("failure lost")
	}
	if _, err := os.Stat("failure"); !os.IsNotExist(err) {
		t.Fatal("partial published output")
	}
}

func TestRealMedia(t *testing.T) {
	for _, tool := range []string{"xorriso", "implantisomd5", "checkisomd5"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skip("real-media tools absent; run container smoke test")
		}
	}
	t.Chdir(t.TempDir())
	o := options{"real fixtures", strings.Repeat("c", 40), "1.0.0", "1", "1"}
	out, err := generate(context.Background(), o, media)
	if err != nil {
		t.Fatal(err)
	}
	if err = command(context.Background(), "checkisomd5", out["iso"]); err != nil {
		t.Fatal(err)
	}
	b, err := exec.Command("xorriso", "-indev", out["iso"], "-report_el_torito", "plain").CombinedOutput()
	if err != nil || !bytes.Contains(b, []byte("BIOS")) || !bytes.Contains(b, []byte("UEFI")) {
		t.Fatalf("missing media entries: %s %v", b, err)
	}
}
