package main

import (
	"os"
	"path/filepath"
	"testing"
)

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
