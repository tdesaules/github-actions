package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestUnreleasedSourcesOnlyRebuildCurrentSelection(t *testing.T) {
	t.Chdir(t.TempDir())
	git := func(args ...string) string {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null", "GIT_AUTHOR_NAME=Fixture", "GIT_AUTHOR_EMAIL=fixture@example.invalid", "GIT_COMMITTER_NAME=Fixture", "GIT_COMMITTER_EMAIL=fixture@example.invalid")
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	write := func(path, data string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(data), 0644); err != nil {
			t.Fatal(err)
		}
	}
	commit := func() string { git("add", "."); git("commit", "-m", "fixture"); return git("rev-parse", "HEAD") }
	git("init", "-b", "main")
	components := []component{}
	for _, name := range []string{"a", "b"} {
		directory := "lint/" + name
		write(directory+"/Containerfile", "FROM scratch AS final\nCOPY shared/"+name+"/ /src/\nCOPY "+directory+"/ /action/\n")
		write(directory+"/action.yml", "metadata\n")
		write("shared/"+name+"/runtime.go", "released\n")
		components = append(components, component{directory, "lint-" + name, "example/" + name + "@sha256:" + strings.Repeat("a", 64)})
	}
	commit()
	git("tag", "1.9.0")
	write("shared/b/runtime.go", "included in latest release\n")
	commit()
	git("tag", "-a", "1.10.0", "-m", "published")
	write("shared/a/runtime.go", "earlier unreleased change; previous run failed\n")
	commit()
	write(".github/workflows/main.yml", "jobs:\n  test:\n    steps: []\n")
	head := commit()
	git("tag", "2.0.0-rc.1")
	unmerged := git("commit-tree", "HEAD^{tree}", "-m", "unrelated release")
	git("tag", "9.0.0", unmerged)
	history, err := unreleasedChanges(head)
	if err != nil {
		t.Fatal(err)
	}
	if history.all || strings.Contains(strings.Join(history.paths, "\n"), "shared/b/runtime.go") {
		t.Fatalf("wrong release baseline: %+v", history)
	}
	for _, tc := range []struct {
		path string
		want map[string]bool
	}{
		{".github/workflows/main.yml", map[string]bool{"lint-a": true, "lint-b": false}},
		{"lint/a/action.yml", map[string]bool{"lint-a": true}},
		{"lint/b/action.yml", map[string]bool{"lint-b": false}},
		{"lint/a/README.md", map[string]bool{}},
	} {
		m, err := selectImages(components, []string{tc.path}, false, history)
		if err != nil {
			t.Fatal(err)
		}
		if len(m.Include) != 2*len(tc.want) {
			t.Fatalf("%s expanded selection: %+v", tc.path, m)
		}
		for _, job := range m.Include {
			want, ok := tc.want[job.Image]
			if !ok || job.Rebuild != want {
				t.Fatalf("%s: %+v", tc.path, job)
			}
		}
	}
	metadataHistory := rebuildHistory{paths: []string{"lint/b/action.yml", "lint/b/README.md"}}
	m, err := selectImages(components, []string{"lint/b/action.yml"}, false, metadataHistory)
	if err != nil || len(m.Include) != 2 || m.Include[0].Rebuild {
		t.Fatalf("metadata/docs-only history rebuilt copied action directory: %+v %v", m, err)
	}
	git("tag", "-d", "1.9.0", "1.10.0", "2.0.0-rc.1", "9.0.0")
	history, err = unreleasedChanges(head)
	if err != nil || !history.all {
		t.Fatalf("missing baseline must be conservative: %+v %v", history, err)
	}
	m, err = selectImages(components, []string{"lint/b/action.yml"}, false, history)
	if err != nil || len(m.Include) != 2 || !m.Include[0].Rebuild {
		t.Fatalf("missing baseline expanded or reused selection: %+v %v", m, err)
	}
	write(".git/shallow", head+"\n")
	if _, err := unreleasedChanges(head); err == nil {
		t.Fatal("shallow history silently accepted")
	}
}

func TestSelection(t *testing.T) {
	t.Chdir(t.TempDir())
	for _, directory := range []string{"lint/a", "lint/b"} {
		if err := os.MkdirAll(directory, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(directory, "Containerfile"), []byte("FROM scratch\nCOPY shared/runtime/ /runtime/\nCOPY LICENSE /LICENSE\n"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	components := []component{{"lint/a", "lint-a", "example/a@sha256:abc"}, {"lint/b", "lint-b", "example/b@sha256:def"}}
	for _, tc := range []struct {
		path    string
		count   int
		rebuild bool
	}{
		{"README.md", 0, false}, {"lint/a/Containerfile", 2, true}, {"lint/a/action.yml", 2, false}, {"shared/runtime/main.go", 4, true}, {".github/workflows/main.yml", 4, false}, {"lint/deleted/Containerfile", 0, false},
		{"tests/fixtures/component/component.json", 4, false},
		{"tests/fixtures/README.md", 0, false},
	} {
		m, err := selectImages(components, []string{tc.path}, false)
		if err != nil {
			t.Fatal(err)
		}
		if len(m.Include) != tc.count {
			t.Fatalf("%s: %+v", tc.path, m)
		}
		for _, job := range m.Include {
			if job.Rebuild != tc.rebuild {
				t.Fatalf("%s: %+v", tc.path, job)
			}
		}
	}
	m, err := selectImages(components, nil, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Include) != 4 {
		t.Fatal("full matrix")
	}
	components[0].reference = "example/a:unpublished"
	m, err = selectImages(components, []string{"lint/a/action.yml"}, false)
	if err != nil {
		t.Fatal(err)
	}
	if !m.Include[0].Rebuild {
		t.Fatal("unpublished image reused")
	}
}

func TestSourceBootstrapDiscoveryAndSelection(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := os.MkdirAll("lint/rust-gtk", 0755); err != nil {
		t.Fatal(err)
	}
	for path, text := range map[string]string{
		"lint/rust-gtk/action.yml":    "runs:\n  using: docker\n  image: Containerfile\n",
		"lint/rust-gtk/Containerfile": "FROM scratch AS final\nCOPY shared/rustgtk/ /src/\nLABEL org.opencontainers.image.title=\"lint-rust-gtk\"\n",
	} {
		if err := os.WriteFile(path, []byte(text), 0644); err != nil {
			t.Fatal(err)
		}
	}
	components, err := discover()
	if err != nil {
		t.Fatal(err)
	}
	for _, paths := range [][]string{{"lint/rust-gtk/action.yml"}, {"shared/rustgtk/deleted.go"}} {
		m, err := selectImages(components, paths, false)
		if err != nil {
			t.Fatal(err)
		}
		if len(m.Include) != 2 {
			t.Fatal(m)
		}
		for _, job := range m.Include {
			if !job.Rebuild || job.Reference != "" {
				t.Fatalf("source was reused or given a pullable reference: %+v", job)
			}
		}
	}
}

func TestSourceDependenciesAndScanSelection(t *testing.T) {
	t.Chdir(t.TempDir())
	for directory, recipe := range map[string]string{
		"lint/a": "FROM scratch AS build\nCOPY --chown=1:1 [\"shared/a/\", \"assets/with spaces.json\", \"/src/\"]\nADD assets/*.tar /opt/\nCOPY LICENSE /LICENSE\nFROM scratch\nCOPY --from=build /src /src\n",
		"lint/b": "FROM scratch\nCOPY shared/b/ /src/\nCOPY LICENSE /LICENSE\nADD https://example.invalid/file /file\n",
	} {
		if err := os.MkdirAll(directory, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(directory+"/Containerfile", []byte(recipe), 0644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(".containerignore", []byte("docs\n**/__pycache__\n"), 0644); err != nil {
		t.Fatal(err)
	}
	components := []component{{"lint/a", "lint-a", "example/a@sha256:abc"}, {"lint/b", "lint-b", "example/b@sha256:def"}}
	for _, tc := range []struct {
		files         []string
		images        int
		rebuild, scan bool
	}{
		{[]string{"shared/a/deleted.go"}, 1, true, true},
		{[]string{"shared/unrelated/main.go"}, 0, false, false},
		{[]string{"LICENSE"}, 2, true, true},
		{[]string{"assets/with spaces.json"}, 1, true, true},
		{[]string{"assets/old.tar", "assets/new.tar"}, 1, true, true},
		{[]string{"grype-review.json"}, 2, false, true},
		{[]string{"lint/a/action.yml"}, 1, false, false},
		{[]string{"README.md", "docs/guide.md"}, 0, false, false},
	} {
		m, err := selectImages(components, tc.files, false)
		if err != nil {
			t.Fatal(err)
		}
		if len(m.Include) != 2*tc.images {
			t.Fatalf("%v: %+v", tc.files, m)
		}
		for _, job := range m.Include {
			if job.Rebuild != tc.rebuild || job.Scan != tc.scan {
				t.Fatalf("%v: %+v", tc.files, job)
			}
		}
	}
}

func TestVariableInputsAreConservativeAndIgnoreContext(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := os.MkdirAll("workflow/a", 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile("workflow/a/Containerfile", []byte("FROM scratch\nARG SOURCE\nCOPY ${SOURCE} /data\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(".containerignore", []byte("docs\n!docs/included.txt\n"), 0644); err != nil {
		t.Fatal(err)
	}
	d, err := buildDependencies("workflow/a")
	if err != nil {
		t.Fatal(err)
	}
	for p, want := range map[string]bool{"LICENSE": true, "shared/unknown.go": true, "docs/ignored.txt": false, "docs/included.txt": true} {
		got, err := d.affected(p)
		if err != nil || got != want {
			t.Fatalf("%s: %t %v", p, got, err)
		}
	}
}

func TestBindAndQuotedSources(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := os.MkdirAll("lint/a", 0755); err != nil {
		t.Fatal(err)
	}
	recipe := "FROM scratch\nCOPY \"assets/with spaces\" /data\nRUN --mount=type=bind,source=shared/fixtures,target=/src true\nRUN --mount=type=cache,target=/cache true\nRUN --mount=type=bind,from=other,source=/external,target=/src true\n"
	if err := os.WriteFile("lint/a/Containerfile", []byte(recipe), 0644); err != nil {
		t.Fatal(err)
	}
	d, err := buildDependencies("lint/a")
	if err != nil {
		t.Fatal(err)
	}
	for path, want := range map[string]bool{"assets/with spaces/file": true, "shared/fixtures/deleted.go": true, "external/file": false, "cache/file": false, "shared/unrelated.go": false} {
		got, err := d.affected(path)
		if err != nil || got != want {
			t.Fatalf("%s: %t %v", path, got, err)
		}
	}
}
