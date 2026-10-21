package main

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestDiffClassification(t *testing.T) {
	t.Chdir(t.TempDir())
	paths := []string{"docs/new name.md", "README.md", "lint/example/Containerfile", ".github/workflows/main.yml", "scripts/no-extension"}
	for _, p := range paths {
		if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("#!/bin/sh\n"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	r := classify([]string{"docs/deleted.md", "docs/new name.md"}, paths, false)
	if !reflect.DeepEqual(r["markdownlint"], []string{"docs/new name.md"}) || len(r["actionlint"]) != 0 {
		t.Fatal(r)
	}
	r = classify([]string{"lychee.toml"}, paths, false)
	if len(r["lychee"]) != 2 || len(r["markdownlint"]) != 0 {
		t.Fatal(r)
	}
	r = classify(nil, paths, true)
	if len(r["shellcheck"]) != 2 || len(r["hadolint"]) != 1 {
		t.Fatal(r)
	}
	for _, selected := range checks([]string{"README.md"}, false) {
		if selected {
			t.Fatal("documentation selected tests")
		}
	}
	if !checks([]string{"shared/testsuite/deleted.go"}, false)["python"] {
		t.Fatal("deleted shared dependency ignored")
	}
}

func TestCheckConditions(t *testing.T) {
	for _, tc := range []struct {
		paths []string
		full  bool
		want  map[string]bool
	}{
		{nil, false, map[string]bool{"catalog": false, "go": false, "python": false}},
		{[]string{"README.md", "check/test-go/README.md"}, false, map[string]bool{"catalog": false, "go": false, "python": false}},
		{[]string{"lint/yaml/deleted_test.go"}, false, map[string]bool{"catalog": false, "go": true, "python": false}},
		{[]string{"scripts/release.py"}, false, map[string]bool{"catalog": false, "go": false, "python": true}},
		{[]string{"lint/yaml/action.yml"}, false, map[string]bool{"catalog": true, "go": false, "python": true}},
		{[]string{"shared/testsuite/testsuite.go"}, false, map[string]bool{"catalog": false, "go": true, "python": true}},
		{[]string{".github/workflows/main.yml"}, false, map[string]bool{"catalog": true, "go": true, "python": true}},
		{nil, true, map[string]bool{"catalog": true, "go": true, "python": true}},
	} {
		if got := checks(tc.paths, tc.full); !reflect.DeepEqual(got, tc.want) {
			t.Fatalf("checks(%v, %t) = %v, want %v", tc.paths, tc.full, got, tc.want)
		}
	}
}
