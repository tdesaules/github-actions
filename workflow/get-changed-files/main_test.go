package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestForcePushBaseRecovery(t *testing.T) {
	root := t.TempDir()
	git := func(directory string, args ...string) string {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", directory}, args...)...)
		cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=Fixture", "GIT_AUTHOR_EMAIL=fixture@example.invalid", "GIT_COMMITTER_NAME=Fixture", "GIT_COMMITTER_EMAIL=fixture@example.invalid")
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	remote := filepath.Join(root, "remote")
	if err := os.Mkdir(remote, 0755); err != nil {
		t.Fatal(err)
	}
	git(remote, "init", "-b", "main")
	if err := os.WriteFile(filepath.Join(remote, "old name.md"), []byte("old\n"), 0644); err != nil {
		t.Fatal(err)
	}
	git(remote, "add", ".")
	git(remote, "commit", "-m", "old branch")
	before := git(remote, "rev-parse", "HEAD")
	git(remote, "checkout", "--orphan", "replacement")
	git(remote, "rm", "-rf", ".")
	if err := os.WriteFile(filepath.Join(remote, "new name.md"), []byte("new\n"), 0644); err != nil {
		t.Fatal(err)
	}
	git(remote, "add", ".")
	git(remote, "commit", "-m", "rewritten branch")
	git(remote, "branch", "-D", "main")
	git(remote, "branch", "-m", "main")
	checkout := filepath.Join(root, "checkout")
	git(root, "clone", "--depth=1", "file://"+remote, checkout)
	t.Chdir(checkout)
	probe := exec.Command("git", "cat-file", "-e", before)
	if probe.Run() == nil {
		t.Fatal("fixture already has unreachable old commit")
	}
	head := git(checkout, "rev-parse", "HEAD")
	paths, full, err := changedPaths(before, head, false)
	if err != nil || full || !reflect.DeepEqual(paths, []string{"new name.md", "old name.md"}) {
		t.Fatalf("recovered diff: %v %t %v", paths, full, err)
	}
	if git(checkout, "rev-parse", "HEAD") != head {
		t.Fatal("fetch moved checkout")
	}
	git(checkout, "remote", "set-url", "origin", filepath.Join(root, "unavailable"))
	if _, full, err := changedPaths(before, head, false); err != nil || full {
		t.Fatal("existing base unnecessarily fetched", err)
	}
	paths, full, err = changedPaths(strings.Repeat("1", 40), head, false)
	if err != nil || !full || len(paths) != 0 {
		t.Fatalf("missing base must select all: %v %t %v", paths, full, err)
	}
	if _, _, err := changedPaths("--upload-pack=invalid", head, false); err == nil {
		t.Fatal("invalid fetch refspec accepted")
	}
}

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
		{[]string{"templates/pantavisor-producer.yml"}, false, map[string]bool{"catalog": false, "go": false, "python": true}},
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

func TestPantavisorFileKinds(t *testing.T) {
	t.Chdir(t.TempDir())
	files := map[string]string{
		"runtime/boot/runtime-layout":     "# shellcheck shell=sh\nroot=/mnt\n",
		"services/core-init":              "#!/bin/sh\nexit 0\n",
		"runtime/boot/udhcpc.script":      "#!/bin/sh\nexit 0\n",
		"build/build.sh":                  "#!/usr/bin/env bash\nexit 0\n",
		"build/Containerfile.tools":       "FROM scratch\n",
		"build/meta-core/service.bb":      "do_install() { :; }\n",
		"build/meta-core/conf/layer.conf": "BBPATH .= \":${LAYERDIR}\"\n",
		"runtime/lxc.conf.in":             "lxc.uts.name = ${node_name}\n",
		"kas/base.yml":                    "header:\n  version: 16\n",
	}
	var paths []string
	for name, content := range files {
		if err := os.MkdirAll(filepath.Dir(name), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(name, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
		paths = append(paths, name)
	}
	got := classify(nil, paths, true)
	if !reflect.DeepEqual(got["shellcheck"], []string{"build/build.sh", "runtime/boot/runtime-layout", "runtime/boot/udhcpc.script", "services/core-init"}) {
		t.Fatal("incorrect shell classification", got)
	}
	if !reflect.DeepEqual(got["hadolint"], []string{"build/Containerfile.tools"}) || !reflect.DeepEqual(got["yaml"], []string{"kas/base.yml"}) || len(got["toml"]) != 0 {
		t.Fatal("incorrect Pantavisor configuration classification", got)
	}
}
