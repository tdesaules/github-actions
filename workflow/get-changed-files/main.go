package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/tdesaules/github-actions/shared/workflow"
)

var shell = regexp.MustCompile(`^#!.*\b(sh|bash|dash|ksh)\b`)
var shellFragment = regexp.MustCompile(`(?m)^# shellcheck shell=(sh|bash|dash|ksh)\b`)

func classify(paths, tracked []string, full bool) map[string][]string {
	result := map[string][]string{}
	for _, name := range []string{"actionlint", "hadolint", "shellcheck", "yaml", "toml", "markdownlint", "lychee", "action-metadata"} {
		result[name] = []string{}
	}
	changed := map[string]bool{}
	for _, p := range paths {
		changed[p] = true
	}
	configs := map[string][]string{"markdownlint": {".markdownlint.json"}, "lychee": {"lychee.toml"}, "hadolint": {".hadolint.yaml", ".hadolint.yml"}, "shellcheck": {".shellcheckrc"}}
	sort.Strings(tracked)
	for _, p := range tracked {
		info, err := os.Stat(p)
		if err != nil || !info.Mode().IsRegular() {
			continue
		}
		name, ext := filepath.Base(p), strings.ToLower(filepath.Ext(p))
		kinds := []string{}
		switch ext {
		case ".yml", ".yaml":
			kinds = append(kinds, "yaml")
			if strings.HasPrefix(p, ".github/workflows/") {
				kinds = append(kinds, "actionlint")
			}
		case ".toml":
			kinds = append(kinds, "toml")
		case ".md", ".markdown":
			kinds = append(kinds, "markdownlint", "lychee")
		case ".sh", ".bash", ".ksh":
			kinds = append(kinds, "shellcheck")
		case "", ".script":
			f, err := os.Open(p)
			if err == nil {
				b := make([]byte, 200)
				n, _ := f.Read(b)
				f.Close()
				if shell.Match(b[:n]) || shellFragment.Match(b[:n]) {
					kinds = append(kinds, "shellcheck")
				}
			}
		}
		if name == "Containerfile" || name == "Dockerfile" || strings.HasPrefix(name, "Containerfile.") || strings.HasPrefix(name, "Dockerfile.") {
			kinds = append(kinds, "hadolint")
		}
		parts := strings.Split(p, "/")
		if name == "action.yml" && len(parts) == 3 && strings.Contains("|oci|check|lint|workflow|maintenance|", "|"+parts[0]+"|") {
			kinds = append(kinds, "action-metadata")
		}
		for _, kind := range kinds {
			selected := full || changed[p]
			for _, config := range configs[kind] {
				selected = selected || changed[config]
			}
			if selected {
				result[kind] = append(result[kind], p)
			}
		}
	}
	return result
}

func checks(paths []string, full bool) map[string]bool {
	r := map[string]bool{"catalog": full, "go": full, "python": full}
	for _, p := range paths {
		ext, name := strings.ToLower(filepath.Ext(p)), filepath.Base(p)
		if ext == ".md" || ext == ".markdown" {
			continue
		}
		if strings.HasPrefix(p, ".github/workflows/") || p == "taskfile.yml" || p == ".containerignore" || p == ".dockerignore" {
			for k := range r {
				r[k] = true
			}
		}
		if name == "action.yml" || name == "Containerfile" || strings.HasPrefix(p, "check/test-catalog/") {
			r["catalog"] = true
		}
		if ext == ".go" || p == "go.mod" || p == "go.sum" || strings.HasPrefix(p, "check/test-go/") {
			r["go"] = true
		}
		if name == "action.yml" || name == "Containerfile" {
			r["python"] = true
		}
		for _, prefix := range []string{"scripts/", "templates/", "tests/unit/", "check/test-python/", "shared/testsuite/"} {
			if strings.HasPrefix(p, prefix) {
				r["python"] = true
			}
		}
	}
	return r
}

// changedPaths preserves an exact diff when a force-push's previous commit can
// still be fetched. If it has disappeared, full selection cannot omit work.
func changedPaths(before, head string, full bool) ([]string, bool, error) {
	if full || before == "" || strings.Trim(before, "0") == "" {
		return []string{}, true, nil
	}
	base, err := workflow.Git("rev-parse", "--verify", "--end-of-options", before+"^{commit}")
	if err != nil {
		// Only event commit IDs may be used as remote fetch arguments. Invalid
		// symbolic revisions remain errors instead of becoming remote refspecs.
		if !regexp.MustCompile(`^[a-fA-F0-9]{40}$`).MatchString(before) {
			return nil, false, fmt.Errorf("invalid or unavailable base revision")
		}
		root, err := os.Getwd()
		if err != nil {
			return nil, false, err
		}
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, "git", "-c", "safe.directory="+root, "fetch", "--no-tags", "--no-recurse-submodules", "--depth=1", "origin", before)
		cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
		cmd.Stdout, cmd.Stderr = os.Stderr, os.Stderr
		if err := cmd.Run(); err != nil {
			fmt.Fprintln(os.Stderr, "Previous commit unavailable; selecting all tracked checks and images")
			return []string{}, true, nil
		}
		base, err = workflow.Git("rev-parse", "--verify", "--end-of-options", before+"^{commit}")
		if err != nil {
			return nil, false, err
		}
	}
	paths, err := workflow.Paths(strings.TrimSpace(base), head)
	return paths, false, err
}

func run() error {
	before := flag.String("before", os.Getenv("INPUT_BEFORE"), "base commit")
	after := flag.String("after", workflow.Default("INPUT_AFTER", "HEAD"), "head commit")
	full := flag.Bool("full", os.Getenv("INPUT_FULL") == "true", "check all tracked files")
	flag.Parse()
	if flag.NArg() != 0 {
		return fmt.Errorf("unexpected arguments")
	}
	// Resolve revisions before passing them to diff, so input cannot become flags.
	head, err := workflow.Git("rev-parse", "--verify", "--end-of-options", *after+"^{commit}")
	if err != nil {
		return err
	}
	paths, selectAll, err := changedPaths(*before, strings.TrimSpace(head), *full)
	if err != nil {
		return err
	}
	*full = selectAll
	tracked, err := workflow.Git("ls-files", "-z")
	if err != nil {
		return err
	}
	for name, files := range classify(paths, workflow.SplitPaths(tracked), *full) {
		if err := workflow.Output(name+"-files", files); err != nil {
			return err
		}
		if err := workflow.Output(name+"-changed", len(files) > 0); err != nil {
			return err
		}
	}
	for name, changed := range checks(paths, *full) {
		if err := workflow.Output(name+"-changed", changed); err != nil {
			return err
		}
	}
	// A literal JSON array is also useful to consumers without any classification.
	data, _ := json.Marshal(paths)
	if err := workflow.Output("files", string(data)); err != nil {
		return err
	}
	return workflow.Output("full", *full)
}

func main() { workflow.Main("get-changed-files 1", run) }
