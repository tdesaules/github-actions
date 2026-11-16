package main

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type fixture struct {
	root    string
	options options
	raw     []byte
	native  descriptor
	blobs   map[string][]byte
	sbom    []byte
}

func jsonBytes(v any) []byte { b, _ := json.Marshal(v); return b }
func newFixture(t *testing.T, arch string) fixture {
	t.Helper()
	t.Setenv("GITHUB_OUTPUT", "")
	f := fixture{root: t.TempDir(), blobs: map[string][]byte{}}
	blob := func(data []byte, media string) descriptor {
		d := descriptor{MediaType: media, Digest: rawDigest(data), Size: int64(len(data))}
		f.blobs[d.Digest] = data
		return d
	}
	cfg := blob(jsonBytes(map[string]any{"architecture": arch, "os": "linux", "config": map[string]any{
		"Env": []string{"PRIVATE_SECRET=do-not-disclose"}, "Labels": map[string]string{
			"org.opencontainers.image.revision": strings.Repeat("a", 40), "private-token": "do-not-disclose"},
	}}), "application/vnd.oci.image.config.v1+json")
	layer := blob([]byte("filesystem layer fixture"), "application/vnd.oci.image.layer.v1.tar")
	f.raw = jsonBytes(manifest{SchemaVersion: 2, MediaType: "application/vnd.oci.image.manifest.v1+json", Config: cfg, Layers: []descriptor{layer}})
	f.native = blob(f.raw, "application/vnd.oci.image.manifest.v1+json")
	f.options = options{Archive: "source archive.oci.tar", Architecture: arch, Revision: strings.Repeat("a", 40), Output: "reports/sbom.json"}
	source := []byte("exact archive transfer fixture")
	if err := os.WriteFile(filepath.Join(f.root, f.options.Archive), source, 0644); err != nil {
		t.Fatal(err)
	}
	f.options.SHA256 = fmt.Sprintf("%x", sha256.Sum256(source))
	f.sbom = spdxFixture("x86_64", true)
	if arch == "arm64" {
		f.sbom = spdxFixture("aarch64", true)
	}
	return f
}
func spdxFixture(arch string, includeInstalled bool) []byte {
	packages := []map[string]any{
		{"name": "nano", "versionInfo": "9.2-1.fc45", "externalRefs": []map[string]string{{"referenceType": "purl", "referenceLocator": "pkg:rpm/fedora/nano@9.2-1.fc45?distro=fedora-45"}}},
		{"name": "example.org/core", "versionInfo": "v1.0.0", "externalRefs": []map[string]string{{"referenceType": "purl", "referenceLocator": "pkg:golang/example.org/core@v1.0.0"}}},
	}
	if includeInstalled {
		packages = append(packages, map[string]any{"name": "niri", "versionInfo": "25.11-1.fc45", "externalRefs": []map[string]string{{"referenceType": "purl", "referenceLocator": "pkg:rpm/fedora/niri@25.11-1.fc45?arch=" + arch}}})
	}
	return jsonBytes(map[string]any{"spdxVersion": "SPDX-2.3", "SPDXID": "SPDXRef-DOCUMENT", "dataLicense": "CC0-1.0",
		"name": "fixture", "documentNamespace": "https://example.test/fixture", "creationInfo": map[string]any{"creators": []string{"Tool: syft-1.54.1"}, "created": "2026-11-16T00:00:00Z"},
		"packages": packages, "files": []map[string]string{{"fileName": "sysroot/ostree/repo/objects/nano.file"}}})
}

type mockTools struct {
	t                          *testing.T
	fixture                    fixture
	calls                      []command
	fail                       string
	tamperLayer, tamperArchive bool
	index                      []byte
	rss                        int64
	expectedAuth               string
	authSeen                   bool
}

func (m *mockTools) run(ctx context.Context, c command) (commandResult, error) {
	m.t.Helper()
	m.calls = append(m.calls, c)
	if c.Name == m.fail {
		return commandResult{}, errors.New("injected tool failure")
	}
	if err := ctx.Err(); err != nil {
		return commandResult{}, err
	}
	if c.Name == "skopeo" {
		op := strings.Join(c.Args, " ")
		if m.expectedAuth != "" {
			var authPath string
			for i, a := range c.Args {
				if a == "--authfile" {
					authPath = c.Args[i+1]
				}
			}
			info, e := os.Stat(authPath)
			if e != nil || info.Mode().Perm() != 0600 {
				m.t.Fatal("private auth file permissions")
			}
			raw, e := os.ReadFile(authPath)
			if e != nil {
				m.t.Fatal(e)
			}
			var auth struct {
				Auths map[string]struct {
					Auth string `json:"auth"`
				} `json:"auths"`
			}
			if json.Unmarshal(raw, &auth) != nil {
				m.t.Fatal("invalid auth file")
			}
			decoded, e := base64.StdEncoding.DecodeString(auth.Auths["ghcr.io"].Auth)
			if e != nil || string(decoded) != m.expectedAuth {
				m.t.Fatal("private auth identity mismatch")
			}
			m.authSeen = true
		}
		if strings.Contains(op, "inspect --raw") {
			if len(m.index) > 0 && strings.Contains(c.Args[len(c.Args)-1], rawDigest(m.index)) {
				return commandResult{Output: m.index}, nil
			}
			return commandResult{Output: m.fixture.raw}, nil
		}
		layout := filepath.Join(c.Work, "layout")
		if err := os.MkdirAll(filepath.Join(layout, "blobs/sha256"), 0700); err != nil {
			m.t.Fatal(err)
		}
		for digest, data := range m.fixture.blobs {
			if m.tamperLayer && string(data) == "filesystem layer fixture" {
				data = []byte("tampered")
			}
			if err := os.WriteFile(filepath.Join(layout, "blobs/sha256", digest[7:]), data, 0600); err != nil {
				m.t.Fatal(err)
			}
		}
		if err := os.WriteFile(filepath.Join(layout, "index.json"), jsonBytes(manifest{SchemaVersion: 2, Manifests: []descriptor{m.fixture.native}}), 0600); err != nil {
			m.t.Fatal(err)
		}
		return commandResult{}, nil
	}
	if c.Name == "umoci" {
		db := filepath.Join(c.Work, "bundle/rootfs/usr/share/rpm")
		if err := os.MkdirAll(db, 0700); err != nil {
			m.t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(db, "rpmdb.sqlite"), []byte("private inventory fixture"), 0600); err != nil {
			m.t.Fatal(err)
		}
		return commandResult{}, nil
	}
	if c.Name == "rpm" {
		arch := "x86_64"
		if m.fixture.options.Architecture == "arm64" {
			arch = "aarch64"
		}
		return commandResult{Output: []byte("niri|0|25.11|1.fc45|" + arch + "\n")}, nil
	}
	if c.Name == "syft" {
		if err := os.WriteFile(filepath.Join(c.Work, "sbom.spdx.json"), m.fixture.sbom, 0600); err != nil {
			m.t.Fatal(err)
		}
		if m.tamperArchive {
			_ = os.WriteFile(filepath.Join(m.fixture.root, m.fixture.options.Archive), []byte("changed"), 0644)
		}
		rss := m.rss
		if rss == 0 {
			rss = 100 << 20
		}
		return commandResult{PeakRSS: rss}, nil
	}
	m.t.Fatalf("unexpected tool %s", c.Name)
	return commandResult{}, nil
}
func assertClean(t *testing.T, m *mockTools) {
	t.Helper()
	for _, c := range m.calls {
		if _, err := os.Stat(c.Work); !os.IsNotExist(err) {
			t.Fatal("private source/rootfs/auth leaked")
		}
	}
}
func TestArchiveBindingRetainsAllSBOMPackagesAndOriginalBytes(t *testing.T) {
	f := newFixture(t, "amd64")
	m := &mockTools{t: t, fixture: f}
	b, o, err := scan(context.Background(), f.root, f.options, "", m.run)
	if err != nil {
		t.Fatal(err)
	}
	if b.Native.Digest != f.native.Digest || b.Source.SHA256 != f.options.SHA256 || b.Architecture != "amd64" || b.Revision != f.options.Revision {
		t.Fatal("source binding mismatch")
	}
	if b.RPM.Installed != 1 || b.RPM.Covered != 1 || b.RPM.Additional != 1 || b.SBOM.Packages != 3 {
		t.Fatal("coverage was narrowed")
	}
	got, err := os.ReadFile(filepath.Join(f.root, o.Output))
	if err != nil || string(got) != string(f.sbom) {
		t.Fatal("SPDX output was filtered or modified")
	}
	if !strings.Contains(string(got), "sysroot/ostree") || !strings.Contains(string(got), "example.org/core") {
		t.Fatal("filesystem evidence lost")
	}
	data, err := os.ReadFile(filepath.Join(f.root, o.Binding))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "do-not-disclose") || strings.Contains(string(data), "rpmdb-query") || strings.Contains(string(data), "/tmp/atomic-sbom-") {
		t.Fatal("private configuration leaked into binding")
	}
	sum, _, err := hashFile(context.Background(), filepath.Join(f.root, o.Output))
	if err != nil || sum != b.SBOM.SHA256 {
		t.Fatal("SBOM checksum mismatch")
	}
	sum, _, err = hashFile(context.Background(), filepath.Join(f.root, o.Archive))
	if err != nil || sum != f.options.SHA256 {
		t.Fatal("input archive changed")
	}
	assertClean(t, m)
}
func TestImmutableIndexStaticCrossArchitectureAndSecretIsolation(t *testing.T) {
	f := newFixture(t, "arm64")
	d := f.native
	d.Platform = &struct {
		Architecture string `json:"architecture"`
		OS           string `json:"os"`
	}{"arm64", "linux"}
	index := jsonBytes(manifest{SchemaVersion: 2, MediaType: "application/vnd.oci.image.index.v1+json", Manifests: []descriptor{d}})
	f.options.Archive = ""
	f.options.SHA256 = ""
	f.options.Reference = "ghcr.io/owner/image@" + rawDigest(index)
	f.options.Username = "actor"
	m := &mockTools{t: t, fixture: f, index: index, expectedAuth: "actor:secret-registry-password"}
	b, o, err := scan(context.Background(), f.root, f.options, "secret-registry-password", m.run)
	if err != nil {
		t.Fatal(err)
	}
	if b.Source.Digest != rawDigest(index) || b.Native.Digest != f.native.Digest || b.Architecture != "arm64" {
		t.Fatal("index/native binding lost")
	}
	data, _ := os.ReadFile(filepath.Join(f.root, o.Binding))
	if strings.Contains(string(data), "secret-registry-password") || strings.Contains(string(data), "actor") {
		t.Fatal("credentials embedded in report")
	}
	if !m.authSeen {
		t.Fatal("registry credentials did not reach private auth file")
	}
	for _, c := range m.calls {
		if strings.Contains(strings.Join(c.Args, " "), "secret-registry-password") {
			t.Fatal("password in tool arguments")
		}
	}
	assertClean(t, m)
}
func TestSourceAndCoverageFailuresPublishNothing(t *testing.T) {
	for _, kind := range []string{"checksum", "architecture", "revision", "blob", "source-changed", "missing-rpm", "syft-failed", "rss", "invalid-spdx"} {
		t.Run(kind, func(t *testing.T) {
			f := newFixture(t, "amd64")
			m := &mockTools{t: t, fixture: f}
			o := f.options
			switch kind {
			case "checksum":
				o.SHA256 = strings.Repeat("0", 64)
			case "architecture":
				o.Architecture = "arm64"
			case "revision":
				o.Revision = strings.Repeat("b", 40)
			case "blob":
				m.tamperLayer = true
			case "source-changed":
				m.tamperArchive = true
			case "missing-rpm":
				m.fixture.sbom = spdxFixture("x86_64", false)
			case "syft-failed":
				m.fail = "syft"
			case "rss":
				m.rss = syftRSSBudget + 1
			case "invalid-spdx":
				m.fixture.sbom = []byte(`{"spdxVersion":"SPDX-2.3"}`)
			}
			if _, _, err := scan(context.Background(), f.root, o, "", m.run); err == nil {
				t.Fatal("invalid scan accepted")
			}
			for _, p := range []string{o.Output, o.Output + ".binding.json"} {
				if _, err := os.Lstat(filepath.Join(f.root, p)); !os.IsNotExist(err) {
					t.Fatal("failed scan left success evidence")
				}
			}
			assertClean(t, m)
		})
	}
}
func TestInvalidPathsAndStaleOutputsAreNotOverwritten(t *testing.T) {
	for _, kind := range []string{"traversal", "absolute", "source-link", "output-link", "binding-alias", "binding-descendant", "existing-sbom", "existing-binding", "mutable-ref"} {
		t.Run(kind, func(t *testing.T) {
			f := newFixture(t, "amd64")
			o := f.options
			sentinel := ""
			switch kind {
			case "traversal":
				o.Output = "../out.json"
			case "absolute":
				o.Archive = "/etc/passwd"
			case "source-link":
				_ = os.Symlink(o.Archive, filepath.Join(f.root, "link"))
				o.Archive = "link"
			case "output-link":
				_ = os.Symlink(t.TempDir(), filepath.Join(f.root, "escape"))
				o.Output = "escape/sbom.json"
			case "binding-alias":
				o.Binding = o.Output
			case "binding-descendant":
				o.Binding = o.Output + "/binding.json"
			case "existing-sbom":
				sentinel = o.Output
			case "existing-binding":
				sentinel = o.Output + ".binding.json"
			case "mutable-ref":
				o.Archive = ""
				o.SHA256 = ""
				o.Reference = "ghcr.io/owner/os:latest"
			}
			if sentinel != "" {
				_ = os.MkdirAll(filepath.Dir(filepath.Join(f.root, sentinel)), 0755)
				_ = os.WriteFile(filepath.Join(f.root, sentinel), []byte("preexisting evidence"), 0644)
			}
			called := false
			_, _, err := scan(context.Background(), f.root, o, "", func(context.Context, command) (commandResult, error) { called = true; return commandResult{}, nil })
			if err == nil || called {
				t.Fatal("invalid input reached a tool")
			}
			if sentinel != "" {
				data, _ := os.ReadFile(filepath.Join(f.root, sentinel))
				if string(data) != "preexisting evidence" {
					t.Fatal("stale/user evidence overwritten")
				}
			}
		})
	}
}
func TestAmbientSettingsCannotFilterPackagesOrExposeCredentials(t *testing.T) {
	t.Setenv("SYFT_EXCLUDE", "**/ostree/**")
	t.Setenv("SYFT_SELECT_CATALOGERS", "rpm-db-cataloger")
	t.Setenv("SYFT_CONFIG", "/untrusted")
	t.Setenv("GITHUB_TOKEN", "private")
	env := strings.Join(toolEnvironment("/private"), "\n")
	for _, forbidden := range []string{"SYFT_EXCLUDE", "SYFT_SELECT_CATALOGERS", "SYFT_CONFIG=", "GITHUB_TOKEN"} {
		if strings.Contains(env, forbidden) {
			t.Fatal("ambient settings inherited")
		}
	}
	if !strings.Contains(env, "GOMEMLIMIT=4GiB") || !strings.Contains(env, "SYFT_PARALLELISM=1") {
		t.Fatal("missing bounded defaults")
	}
	if _, err := parse([]string{"--operation=run"}); err == nil {
		t.Fatal("operation selector accepted")
	}
	if _, err := parse([]string{"--registry-password=secret"}); err == nil {
		t.Fatal("secret CLI input accepted")
	}
	if _, err := parse([]string{"--registry-username=actor"}); err == nil {
		t.Fatal("credential CLI input accepted")
	}
}
