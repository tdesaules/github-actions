package atomicrelease

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func completeChecks() map[string]string {
	return map[string]string{
		"Fedora version": "", "Niri configuration": "", "desktop and keyring packages": "",
		"PAM dynamic dependencies": "", "enabled greeter": "", "enabled GPU profile": "",
		"GPU helper syntax": "", "GPU memory ceiling": "",
	}
}

func TestReceiptRequiresEveryFunctionalCheck(t *testing.T) {
	_, b, _ := fixture(t)
	validate := func(r Receipt) error {
		return ValidateReceipt(r, "os", "amd64", r.SourceSHA, r.Version, "42", "1")
	}
	if err := validate(b.AMD64); err != nil {
		t.Fatalf("empty command output is valid success evidence: %v", err)
	}
	for name := range completeChecks() {
		t.Run(name, func(t *testing.T) {
			r := b.AMD64
			r.Checks = completeChecks()
			delete(r.Checks, name)
			r.Checks["arbitrary check"] = "passed"
			if validate(r) == nil {
				t.Fatal("missing required check accepted despite unchanged map size")
			}
		})
	}
	r := b.AMD64
	r.Checks = map[string]string{"arbitrary check": "passed"}
	if validate(r) == nil {
		t.Fatal("tested marker plus arbitrary check accepted")
	}
	r = b.AMD64
	r.BootTested = true
	if validate(r) == nil {
		t.Fatal("userspace receipt claimed boot testing")
	}
}

func TestPayloadRequiresISOBuildReceiptName(t *testing.T) {
	_, b, files := fixture(t)
	if err := ValidatePayloads(files, b.ISO); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"iso-build.json", "other.iso.build.json", "qualification.json", "iso-qualification.json"} {
		t.Run(name, func(t *testing.T) {
			bad := append([]Payload(nil), files...)
			for i := range bad {
				if bad[i].MediaType == "application/json" {
					bad[i].Path = name
				}
			}
			if ValidatePayloads(bad, b.ISO) == nil {
				t.Fatal("unbound JSON basename accepted")
			}
		})
	}
}

func TestManifestTypesAreBlocking(t *testing.T) {
	e := evidenceFixture(t)
	index := manifest{}
	for _, r := range []Receipt{e.Qualified.AMD64, e.Qualified.ARM64} {
		d := descriptor{Digest: r.ManifestDigest}
		d.Platform.Architecture, d.Platform.OS = r.Arch, "linux"
		index.Manifests = append(index.Manifests, d)
	}
	for _, typ := range []string{"", "application/json", "application/vnd.oci.image.manifest.v1+json",
		"application/vnd.oci.image.index.v1+json", "application/vnd.docker.distribution.manifest.list.v2+json"} {
		index.MediaType = typ
		raw, _ := json.Marshal(index)
		err := verifyIndex(raw, e.Qualified)
		want := typ == "application/vnd.oci.image.index.v1+json" || typ == "application/vnd.docker.distribution.manifest.list.v2+json"
		if (err == nil) != want {
			t.Fatalf("index type %q: %v", typ, err)
		}
	}
	iso := manifest{}
	for _, f := range e.ISOFiles {
		d := descriptor{Digest: "sha256:" + f.SHA256, MediaType: f.MediaType,
			Annotations: map[string]string{"org.opencontainers.image.title": f.Path}}
		if f.Path == e.ISOFile {
			d.Size = e.Qualified.ISO.Size
		}
		iso.Layers = append(iso.Layers, d)
	}
	for _, typ := range []string{"", "application/json", "application/vnd.niri-tonic.installer.v2", "application/vnd.niri-tonic.installer.v1"} {
		iso.ArtifactType = typ
		raw, _ := json.Marshal(iso)
		err := verifyISO(raw, e.Qualified.ISO, e.ISOFiles)
		if (err == nil) != (typ == "application/vnd.niri-tonic.installer.v1") {
			t.Fatalf("installer artifact type %q: %v", typ, err)
		}
	}
}

func TestReleaseCreationRequiresURL(t *testing.T) {
	e := evidenceFixture(t)
	for _, body := range []string{`{}`, `{"html_url":null}`, `{"html_url":""}`, `{"html_url":"  "}`} {
		t.Run(body, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == "GET" {
					w.WriteHeader(404)
					return
				}
				w.WriteHeader(201)
				_, _ = w.Write([]byte(body))
			}))
			defer server.Close()
			a, _ := NewRemote("owner/os", "fixture", "", "")
			a.APIURL, a.Client = server.URL, server.Client()
			if url, err := a.Release(e, strings.Repeat("f", 40), "notes", true); err == nil || url != "" {
				t.Fatalf("incomplete success response accepted: %q, %v", url, err)
			}
		})
	}
}

func TestMetadataRejectsUnsafeTargetsBeforeEitherWrite(t *testing.T) {
	for _, name := range []string{"CHANGELOG.md", ".github/atomic-release.json"} {
		for _, kind := range []string{"symlink", "dangling-symlink", "directory"} {
			t.Run(name+"/"+kind, func(t *testing.T) {
				initRepo(t)
				if err := writeReleaseMetadata(Evidence{}, "original notes"); err != nil {
					t.Fatal(err)
				}
				runGit(t, "add", "--", "CHANGELOG.md", ".github/atomic-release.json")
				other := "CHANGELOG.md"
				if name == other {
					other = ".github/atomic-release.json"
				}
				before, err := os.ReadFile(other)
				if err != nil {
					t.Fatal(err)
				}
				target := filepath.Join(t.TempDir(), "target")
				if kind != "dangling-symlink" {
					if err := os.WriteFile(target, []byte("untouched"), 0644); err != nil {
						t.Fatal(err)
					}
				}
				if err := os.Remove(name); err != nil {
					t.Fatal(err)
				}
				if kind == "directory" {
					err = os.Mkdir(name, 0755)
				} else {
					err = os.Symlink(target, name)
				}
				if err != nil {
					t.Fatal(err)
				}
				if writeReleaseMetadata(Evidence{}, "replacement") == nil {
					t.Fatal("unsafe metadata accepted")
				}
				after, err := os.ReadFile(other)
				if err != nil || string(after) != string(before) {
					t.Fatal("other metadata changed before rejection")
				}
				if kind == "dangling-symlink" {
					if _, err := os.Lstat(target); !os.IsNotExist(err) {
						t.Fatal("dangling symlink target was created")
					}
				} else if got, err := os.ReadFile(target); err != nil || string(got) != "untouched" {
					t.Fatal("external target changed")
				}
			})
		}
	}
	t.Run("symlinked-parent", func(t *testing.T) {
		initRepo(t)
		target := t.TempDir()
		if err := os.Symlink(target, ".github"); err != nil {
			t.Fatal(err)
		}
		if writeReleaseMetadata(Evidence{}, "notes") == nil {
			t.Fatal("symlinked evidence parent accepted")
		}
		entries, err := os.ReadDir(target)
		if err != nil || len(entries) != 0 {
			t.Fatal("symlinked directory was modified")
		}
		if _, err := os.Lstat("CHANGELOG.md"); !os.IsNotExist(err) {
			t.Fatal("changelog written before rejecting parent")
		}
	})
	t.Run("tracked-regular-files", func(t *testing.T) {
		initRepo(t)
		if err := writeReleaseMetadata(Evidence{}, "original"); err != nil {
			t.Fatal(err)
		}
		runGit(t, "add", "--", "CHANGELOG.md", ".github/atomic-release.json")
		if err := writeReleaseMetadata(Evidence{Version: "1.0.0"}, "updated"); err != nil {
			t.Fatal(err)
		}
		got, err := os.ReadFile("CHANGELOG.md")
		if err != nil || string(got) != "updated\n" {
			t.Fatal("regular metadata update failed")
		}
	})
}
