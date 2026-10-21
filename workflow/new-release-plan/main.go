package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/tdesaules/github-actions/shared/workflow"
)

var stable = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$`)

func latest(tags string) string {
	versions := []string{}
	for _, tag := range strings.Fields(tags) {
		if stable.MatchString(tag) {
			versions = append(versions, tag)
		}
	}
	sort.Slice(versions, func(i, j int) bool {
		a, b := strings.Split(versions[i], "."), strings.Split(versions[j], ".")
		for k := 0; k < 3; k++ {
			if len(a[k]) != len(b[k]) {
				return len(a[k]) < len(b[k])
			}
			if a[k] != b[k] {
				return a[k] < b[k]
			}
		}
		return false
	})
	if len(versions) == 0 {
		return ""
	}
	return versions[len(versions)-1]
}
func run() error {
	config := flag.String("config", workflow.Default("INPUT_CONFIG", "cliff.toml"), "git-cliff configuration")
	output := flag.String("output", workflow.Default("INPUT_OUTPUT", ".build/publication/release-plan.json"), "plan path")
	flag.Parse()
	if flag.NArg() != 0 {
		return fmt.Errorf("unexpected arguments")
	}
	base, err := workflow.Git("rev-parse", "HEAD")
	if err != nil {
		return err
	}
	base = strings.TrimSpace(base)
	tags, err := workflow.Git("tag", "--merged", "HEAD")
	if err != nil {
		return err
	}
	previous := latest(tags)
	rangeRef := "HEAD"
	if previous != "" {
		rangeRef = previous + "..HEAD"
	}
	subjects, err := workflow.Git("log", "--format=%s", rangeRef, "--")
	if err != nil {
		return err
	}
	changed := false
	for _, s := range strings.Split(strings.TrimSpace(subjects), "\n") {
		if s != "" && !strings.HasPrefix(s, "chore(release):") {
			changed = true
		}
	}
	version := previous
	if changed {
		// libgit2 (used by git-cliff) does not inherit Git CLI -c options.
		// Trust only this checkout in a private, disposable configuration.
		home, err := os.MkdirTemp("", "release-plan-git-")
		if err != nil {
			return err
		}
		defer os.RemoveAll(home)
		root, err := os.Getwd()
		if err != nil {
			return err
		}
		gitConfig := filepath.Join(home, ".gitconfig")
		configure := exec.Command("git", "config", "--file", gitConfig, "--add", "safe.directory", root)
		configure.Stderr = os.Stderr
		if err := configure.Run(); err != nil {
			return err
		}
		cmd := exec.Command("git-cliff", "--config", *config, "--bumped-version")
		cmd.Env = append(os.Environ(), "HOME="+home, "XDG_CONFIG_HOME="+home, "GIT_CONFIG_GLOBAL="+gitConfig)
		cmd.Stderr = os.Stderr
		data, err := cmd.Output()
		if err != nil {
			return err
		}
		version = strings.TrimSpace(string(data))
	}
	if !stable.MatchString(version) {
		return fmt.Errorf("git-cliff did not produce stable SemVer")
	}
	changed = changed && version != previous
	var previousValue any
	if previous != "" {
		previousValue = previous
	}
	data, err := json.MarshalIndent(map[string]any{"base": base, "previous": previousValue, "version": version, "changed": changed}, "", "  ")
	if err != nil {
		return err
	}
	if err = os.MkdirAll(filepath.Dir(*output), 0755); err != nil {
		return err
	}
	if err = os.WriteFile(*output, append(data, '\n'), 0644); err != nil {
		return err
	}
	if err = workflow.Output("release-version", version); err != nil {
		return err
	}
	return workflow.Output("release-changed", strconv.FormatBool(changed))
}
func main() { workflow.Main("new-release-plan 1", run) }
