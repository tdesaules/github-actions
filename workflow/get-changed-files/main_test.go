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
