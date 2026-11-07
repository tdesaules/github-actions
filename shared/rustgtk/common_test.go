package rustgtk

import (
	"archive/tar"
	"compress/gzip"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestReceiptRejectsTampering(t *testing.T) {
	v := Inputs{Version: "0.1.0", SourceSHA: strings.Repeat("a", 40), Arch: runtime.GOARCH}
	v.Archive = filepath.Join(t.TempDir(), ArchiveName(v))
	if e := os.WriteFile(v.Archive, []byte("candidate"), 0644); e != nil {
		t.Fatal(e)
	}
	h, _ := Hash(v.Archive)
	r := Receipt{SchemaVersion: 1, Version: v.Version, SourceSHA: v.SourceSHA, Component: Component, Arch: v.Arch, Archive: filepath.Base(v.Archive), SHA256: h, RunID: "123", RunAttempt: "1"}
	t.Setenv("GITHUB_RUN_ID", "123")
	t.Setenv("GITHUB_RUN_ATTEMPT", "1")
	if e := Verify(v, r); e != nil {
		t.Fatal(e)
	}
	for _, mutate := range []func(*Receipt){func(r *Receipt) { r.Tested = true }, func(r *Receipt) { r.SourceSHA = strings.Repeat("b", 40) }, func(r *Receipt) { r.SHA256 = strings.Repeat("0", 64) }, func(r *Receipt) { r.Arch = "wrong" }, func(r *Receipt) { r.RunID = "456" }, func(r *Receipt) { r.Archive = "other.tar.gz" }} {
		bad := r
		mutate(&bad)
		if Verify(v, bad) == nil {
			t.Fatal("accepted mismatched receipt")
		}
	}
}

func TestReceiptSourceModesAndProvenance(t *testing.T) {
	t.Setenv("GITHUB_RUN_ID", "900")
	t.Setenv("GITHUB_RUN_ATTEMPT", "2")
	for _, source := range []string{"candidate", "published"} {
		t.Run(source, func(t *testing.T) {
			v := Inputs{Version: "0.1.0", SourceSHA: strings.Repeat("a", 40), Arch: runtime.GOARCH, Source: source}
			v.Archive = filepath.Join(t.TempDir(), ArchiveName(v))
			if err := os.WriteFile(v.Archive, []byte("exact released payload"), 0644); err != nil {
				t.Fatal(err)
			}
			hash, err := Hash(v.Archive)
			if err != nil {
				t.Fatal(err)
			}
			r := Receipt{SchemaVersion: 1, Version: v.Version, SourceSHA: v.SourceSHA, Component: Component, Arch: v.Arch, Archive: filepath.Base(v.Archive), SHA256: hash, RunID: "900", RunAttempt: "2"}
			if source == "published" {
				v.SourceRunID, v.SourceRunAttempt = "100", "3"
				r.RunID, r.RunAttempt, r.Tested = "100", "3", true
			}
			if err := Verify(v, r); err != nil {
				t.Fatal(err)
			}
			for name, mutate := range map[string]func(*Inputs, *Receipt){
				"wrong mode":         func(v *Inputs, r *Receipt) { v.Source = "other" },
				"wrong tested state": func(v *Inputs, r *Receipt) { r.Tested = !r.Tested },
				"wrong run":          func(v *Inputs, r *Receipt) { r.RunID = "999" },
				"wrong attempt":      func(v *Inputs, r *Receipt) { r.RunAttempt = "999" },
				"missing run":        func(v *Inputs, r *Receipt) { r.RunID = "" },
				"missing attempt":    func(v *Inputs, r *Receipt) { r.RunAttempt = "" },
				"wrong hash":         func(v *Inputs, r *Receipt) { r.SHA256 = strings.Repeat("0", 64) },
				"wrong source":       func(v *Inputs, r *Receipt) { r.SourceSHA = strings.Repeat("b", 40) },
				"wrong version":      func(v *Inputs, r *Receipt) { r.Version = "9.9.9" },
				"wrong arch":         func(v *Inputs, r *Receipt) { r.Arch = "wrong" },
			} {
				t.Run(name, func(t *testing.T) {
					badV, badR := v, r
					mutate(&badV, &badR)
					if Verify(badV, badR) == nil {
						t.Fatal("accepted invalid provenance")
					}
				})
			}
			if source == "published" {
				for _, field := range []string{"id", "attempt"} {
					bad := v
					if field == "id" {
						bad.SourceRunID = ""
					} else {
						bad.SourceRunAttempt = ""
					}
					if Verify(bad, r) == nil {
						t.Fatal("published mode accepted missing expected", field)
					}
				}
				bad := v
				bad.SourceRunID, bad.SourceRunAttempt = "900", "2"
				if Verify(bad, r) == nil {
					t.Fatal("published mode replaced original provenance with current run")
				}
			} else {
				bad := v
				bad.SourceRunID, bad.SourceRunAttempt = r.RunID, r.RunAttempt
				if Verify(bad, r) == nil {
					t.Fatal("candidate accepted source-run overrides")
				}
				for _, env := range []string{"GITHUB_RUN_ID", "GITHUB_RUN_ATTEMPT"} {
					t.Run("missing "+env, func(t *testing.T) {
						t.Setenv(env, "")
						if Verify(v, r) == nil {
							t.Fatal("accepted missing current provenance")
						}
					})
				}
			}
			if err := os.WriteFile(v.Archive, []byte("tampered payload"), 0644); err != nil {
				t.Fatal(err)
			}
			if Verify(v, r) == nil {
				t.Fatal("accepted modified archive")
			}
		})
	}
}
func TestExtractRejectsTraversalAndLinks(t *testing.T) {
	for _, h := range []tar.Header{{Name: "../escape", Typeflag: tar.TypeReg}, {Name: "bin/app", Typeflag: tar.TypeSymlink, Linkname: "/etc/passwd"}, {Name: "/absolute", Typeflag: tar.TypeReg}} {
		t.Run(h.Name, func(t *testing.T) {
			p := filepath.Join(t.TempDir(), "bad.gz")
			f, _ := os.Create(p)
			gz := gzip.NewWriter(f)
			tw := tar.NewWriter(gz)
			if e := tw.WriteHeader(&h); e != nil {
				t.Fatal(e)
			}
			tw.Close()
			gz.Close()
			f.Close()
			if Extract(p, t.TempDir()) == nil {
				t.Fatal("unsafe extraction succeeded")
			}
		})
	}
}
func TestPackageReproducibleAndExcludesPrivateFiles(t *testing.T) {
	root := t.TempDir()
	for _, p := range []string{"app", "Cargo.lock", "data/io.github.tdesaules.NiriTonicFiles.desktop", "data/io.github.tdesaules.NiriTonicFiles.svg", "LICENSE", "README.md", "config.example.yaml", "docs/start.md", "plugins/filen/adapter.mjs", "plugins/filen/package-lock.json", "plugins/google-drive/adapter.mjs", "plugins/filen/config.yaml", "plugins/filen/node_modules/secret.mjs", "config.yaml"} {
		dest := filepath.Join(root, p)
		os.MkdirAll(filepath.Dir(dest), 0755)
		os.WriteFile(dest, []byte(p), 0644)
	}
	commitFixture(t, root)
	for _, p := range []string{"docs/injected-secret.md", "plugins/filen/injected-secret.mjs"} {
		if err := os.WriteFile(filepath.Join(root, p), []byte("private credentials"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	a, b := filepath.Join(t.TempDir(), "a.gz"), filepath.Join(t.TempDir(), "b.gz")
	for _, p := range []string{a, b} {
		if e := Package(root, filepath.Join(root, "app"), p, 123); e != nil {
			t.Fatal(e)
		}
	}
	ha, _ := Hash(a)
	hb, _ := Hash(b)
	if ha != hb {
		t.Fatal("archive is not reproducible")
	}
	dest := t.TempDir()
	if e := Extract(a, dest); e != nil {
		t.Fatal(e)
	}
	filepath.WalkDir(dest, func(p string, d os.DirEntry, e error) error {
		if e != nil {
			t.Fatal(e)
		}
		if strings.Contains(p, "node_modules") || d.Name() == "config.yaml" || strings.Contains(d.Name(), "injected-secret") {
			t.Fatal("private data packaged", p)
		}
		return nil
	})
	if content, err := os.ReadFile(filepath.Join(dest, "share/doc", Component, "Cargo.lock")); err != nil || string(content) != "Cargo.lock" {
		t.Fatalf("missing original lockfile: %s, %v", content, err)
	}
}

func commitFixture(t *testing.T, root string) string {
	t.Helper()
	for _, args := range [][]string{{"init"}, {"add", "."}, {"-c", "user.name=Fixture", "-c", "user.email=fixture@example.invalid", "-c", "commit.gpgsign=false", "commit", "-m", "fixture"}} {
		if _, err := sourceGit(root, args...); err != nil {
			t.Fatal(err)
		}
	}
	sha, err := sourceGit(root, "rev-parse", "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(sha))
}

func TestSourceRejectsTrackedAndIndexChanges(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "Cargo.lock")
	if err := os.WriteFile(file, []byte("original"), 0644); err != nil {
		t.Fatal(err)
	}
	sha := commitFixture(t, root)
	t.Chdir(root)
	if err := Source(sha); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(".build/payload", 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(".build/payload/generated", []byte("output"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := Source(sha); err != nil {
		t.Fatalf("generated output rejected: %v", err)
	}
	if err := os.WriteFile(file, []byte("changed"), 0644); err != nil {
		t.Fatal(err)
	}
	if Source(sha) == nil {
		t.Fatal("accepted tracked worktree change")
	}
	if _, err := sourceGit(root, "add", "Cargo.lock"); err != nil {
		t.Fatal(err)
	}
	if Source(sha) == nil {
		t.Fatal("accepted staged change")
	}
	if err := os.WriteFile(file, []byte("original"), 0644); err != nil {
		t.Fatal(err)
	}
	if Source(sha) == nil {
		t.Fatal("accepted staged change with worktree restored to HEAD")
	}
}
