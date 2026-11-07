package binaryrelease

import (
	"path/filepath"
	"sort"
	"strings"
)

func runtimePath(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	if ext == ".md" || ext == ".markdown" {
		return false
	}
	for _, prefix := range []string{"src/", "tests/", "scripts/", "plugins/", "config/", "themes/", "data/", "runtime/", ".github/workflows/", ".github/scripts/"} {
		if strings.HasPrefix(path, prefix) {
			return true
		}
	}
	return strings.HasPrefix(path, "Cargo.") || path == "Makefile" || path == "build.rs" || path == "config.example.yaml" || path == "LICENSE" || strings.HasPrefix(path, ".cargo/")
}

func lintKinds(path string) []string {
	kinds := []string{}
	if runtimePath(path) && !strings.HasPrefix(path, ".github/workflows/") {
		kinds = append(kinds, "rust-gtk")
	}
	ext := strings.ToLower(filepath.Ext(path))
	if ext == ".yaml" || ext == ".yml" {
		kinds = append(kinds, "yaml")
		if strings.HasPrefix(path, ".github/workflows/") {
			kinds = append(kinds, "actionlint")
		}
	}
	if ext == ".toml" || path == "Cargo.lock" {
		kinds = append(kinds, "toml")
	}
	if ext == ".md" || ext == ".markdown" {
		kinds = append(kinds, "markdownlint")
	}
	return kinds
}

// selectLints uses tracked files at the intended revision. Deleted paths select
// validation of remaining matching files, but never enter a linter's file list.
func selectLints(source string, changed []string, full bool) (Matrix, error) {
	raw, e := gitRaw("ls-tree", "-r", "--name-only", "-z", source)
	if e != nil {
		return matrix(), e
	}
	tracked := map[string]bool{}
	all := map[string][]string{}
	for _, path := range strings.Split(strings.TrimSuffix(string(raw), "\x00"), "\x00") {
		if path == "" {
			continue
		}
		tracked[path] = true
		for _, kind := range lintKinds(path) {
			all[kind] = append(all[kind], path)
		}
	}
	selected := map[string]map[string]bool{}
	add := func(kind, path string) {
		if selected[kind] == nil {
			selected[kind] = map[string]bool{}
		}
		selected[kind][path] = true
	}
	selectAll := func(kind string) {
		for _, path := range all[kind] {
			add(kind, path)
		}
	}
	if full {
		for kind := range all {
			selectAll(kind)
		}
	} else {
		for _, path := range changed {
			switch path {
			case ".markdownlint.json", ".markdownlint.yaml", ".markdownlint.yml", ".markdownlint-cli2.jsonc":
				selectAll("markdownlint")
			case ".yamllint", ".yamllint.yml", ".yamllint.yaml":
				selectAll("yaml")
			case ".github/actionlint.yaml", ".github/actionlint.yml":
				selectAll("actionlint")
			}
			for _, kind := range lintKinds(path) {
				if !tracked[path] || kind == "rust-gtk" {
					selectAll(kind)
				} else {
					add(kind, path)
				}
			}
		}
	}
	result := matrix()
	for _, id := range []string{"rust-gtk", "actionlint", "yaml", "toml", "markdownlint"} {
		files := []string{}
		for path := range selected[id] {
			files = append(files, path)
		}
		sort.Strings(files)
		if len(files) > 0 {
			result.Include = append(result.Include, map[string]any{"id": id, "action_path": "lint/" + id, "runner": "ubuntu-24.04", "files": files})
		}
	}
	return result, nil
}
