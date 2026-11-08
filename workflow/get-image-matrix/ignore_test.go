package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tdesaules/github-actions/shared/workflow"
)

func sourceFixture(t *testing.T) (func(...string) string, func(string, string)) {
	t.Helper()
	t.Chdir(t.TempDir())
	for k, v := range map[string]string{"GIT_CONFIG_NOSYSTEM": "1", "GIT_CONFIG_GLOBAL": "/dev/null", "GIT_AUTHOR_NAME": "Fixture", "GIT_AUTHOR_EMAIL": "fixture@example.invalid", "GIT_COMMITTER_NAME": "Fixture", "GIT_COMMITTER_EMAIL": "fixture@example.invalid"} {
		t.Setenv(k, v)
	}
	git := func(args ...string) string {
		t.Helper()
		out, err := workflow.Git(args...)
		if err != nil {
			t.Fatal(err)
		}
		return strings.TrimSpace(out)
	}
	write := func(path, content string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}
	git("init", "-b", "main")
	for _, name := range []string{"a", "b"} {
		write("oci/"+name+"/Containerfile", "FROM scratch AS final\nCOPY oci/"+name+"/ /app/\n")
		write("oci/"+name+"/main.go", "released\n")
		write("oci/"+name+"/asset", "asset\n")
	}
	return git, write
}

func TestIgnoreChangesOnlyInvalidateEffectiveInputs(t *testing.T) {
	for _, tc := range []struct {
		name, before, after string
		wantA               bool
	}{
		{"retired temporary directory", ".source-actions\n", "", false},
		{"comments", "# old\n", "# new\n", false},
		{"exclude copied input", "", "oci/a/asset\n", true},
		{"expose copied input", "oci/a/asset\n", "", true},
		{"negated input", "oci/a/*\n!oci/a/main.go\n", "oci/a/*\n!oci/a/main.go\n!oci/a/asset\n", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			git, write := sourceFixture(t)
			write(".containerignore", tc.before)
			write(".dockerignore", tc.before)
			git("add", ".")
			git("commit", "-m", "release")
			git("tag", "1.0.0")
			write(".containerignore", tc.after)
			write(".dockerignore", tc.after)
			git("add", ".")
			git("commit", "-m", "ignore change")
			history, err := unreleasedChanges(git("rev-parse", "HEAD"))
			if err != nil {
				t.Fatal(err)
			}
			components := []component{{"oci/a", "oci-a", "example@sha256:" + strings.Repeat("a", 64)}, {"oci/b", "oci-b", "example@sha256:" + strings.Repeat("b", 64)}}
			for _, paths := range [][]string{{".containerignore", ".dockerignore"}, {".github/workflows/main.yml"}} {
				m, err := selectImages(components, paths, false, history)
				if err != nil {
					t.Fatal(err)
				}
				wantCount := 4
				if paths[0] == ".containerignore" {
					wantCount = 0
					if tc.wantA {
						wantCount = 2
					}
				}
				if len(m.Include) != wantCount {
					t.Fatalf("qualification selection lost: %+v", m)
				}
				for _, j := range m.Include {
					want := j.Image == "oci-a" && tc.wantA
					if j.Rebuild != want {
						t.Fatalf("%s: %+v want rebuild=%v", tc.name, j, want)
					}
				}
			}
		})
	}
}

func TestIgnoreFileCopiedAsDataStillInvalidates(t *testing.T) {
	git, write := sourceFixture(t)
	write("oci/a/Containerfile", "FROM scratch AS final\nCOPY . /app/\n")
	write(".containerignore", "# old\n")
	git("add", ".")
	git("commit", "-m", "base")
	base := git("rev-parse", "HEAD")
	write(".containerignore", "# new\n")
	git("add", ".")
	git("commit", "-m", "changed")
	change, err := loadIgnoreChange(base, []string{".containerignore"})
	if err != nil {
		t.Fatal(err)
	}
	deps, err := buildDependencies("oci/a")
	if err != nil {
		t.Fatal(err)
	}
	if affected, err := change.affects(deps); err != nil || !affected {
		t.Fatalf("copied ignore bytes must invalidate: %v %v", affected, err)
	}
}
