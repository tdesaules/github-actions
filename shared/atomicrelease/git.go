package atomicrelease

import (
	"encoding/json"
	"fmt"
	"math/big"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/tdesaules/github-actions/shared/workflow"
)

func git(args ...string) (string, error) {
	s, e := workflow.Git(args...)
	return strings.TrimSpace(s), e
}
func Compare(a, b string) int {
	x, y := strings.Split(a, "."), strings.Split(b, ".")
	for i := 0; i < 3; i++ {
		if len(x[i]) < len(y[i]) {
			return -1
		}
		if len(x[i]) > len(y[i]) {
			return 1
		}
		if c := strings.Compare(x[i], y[i]); c != 0 {
			return c
		}
	}
	return 0
}
func Bump(messages string) string {
	level := 0
	for _, m := range strings.Split(messages, "\x00") {
		m = strings.TrimSpace(m)
		subject := strings.SplitN(m, "\n", 2)[0]
		colon := strings.Index(subject, ":")
		if colon < 0 {
			continue
		}
		typ := strings.ToLower(subject[:colon])
		if strings.HasSuffix(typ, "!") || strings.Contains(m, "\nBREAKING CHANGE:") || strings.Contains(m, "\nBREAKING-CHANGE:") {
			return "major"
		}
		if i := strings.Index(typ, "("); i >= 0 {
			typ = typ[:i]
		}
		if typ == "feat" {
			level = 2
		} else if (typ == "fix" || typ == "perf") && level < 1 {
			level = 1
		}
	}
	return []string{"", "patch", "minor"}[level]
}
func cliff(args ...string) (string, error) {
	home, err := os.MkdirTemp("", "atomic-cliff-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(home)
	root, err := os.Getwd()
	if err != nil {
		return "", err
	}
	config := filepath.Join(home, ".gitconfig")
	cmd := exec.Command("git", "config", "--file", config, "--add", "safe.directory", root)
	if err = cmd.Run(); err != nil {
		return "", err
	}
	cmd = exec.Command("git-cliff", append([]string{"--offline", "--no-exec"}, args...)...)
	cmd.Env = append(os.Environ(), "HOME="+home, "XDG_CONFIG_HOME="+home, "GIT_CONFIG_GLOBAL="+config)
	data, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git-cliff failed")
	}
	return strings.TrimSpace(string(data)), nil
}

// ReleaseVersion uses fixed git-cliff bump policy independently of note filtering.
func ReleaseVersion(initial bool) (string, bool, error) {
	shallow, err := git("rev-parse", "--is-shallow-repository")
	if err != nil {
		return "", false, err
	}
	if shallow != "false" {
		return "", false, fmt.Errorf("full release history and tags required")
	}
	tags, err := git("tag", "--merged", "HEAD")
	if err != nil {
		return "", false, err
	}
	previous := ""
	for _, t := range strings.Fields(tags) {
		if Stable.MatchString(t) && (previous == "" || Compare(t, previous) > 0) {
			previous = t
		}
	}
	if previous == "" {
		if initial {
			return "1.0.0", true, nil
		}
		return "", false, nil
	}
	if initial {
		return "", false, fmt.Errorf("initial-release requires no previous stable release")
	}
	messages, err := git("log", previous+"..HEAD", "--format=%B%x00")
	if err != nil {
		return "", false, err
	}
	level := Bump(messages)
	if level == "" {
		return previous, false, nil
	}
	if strings.HasPrefix(previous, "0.") && level == "major" {
		level = "minor"
	}
	parts := strings.Split(previous, ".")
	index := map[string]int{"major": 0, "minor": 1, "patch": 2}[level]
	n, _ := new(big.Int).SetString(parts[index], 10)
	parts[index] = n.Add(n, big.NewInt(1)).String()
	for i := index + 1; i < 3; i++ {
		parts[i] = "0"
	}
	expected := strings.Join(parts, ".")
	f, err := os.CreateTemp("", "atomic-bump-*.toml")
	if err != nil {
		return "", false, err
	}
	defer os.Remove(f.Name())
	_, err = f.WriteString("[git]\nconventional_commits = false\nfilter_unconventional = false\nfilter_commits = false\ntag_pattern = '^[0-9]+\\.[0-9]+\\.[0-9]+$'\ncommit_parsers = [{ message = '.*', group = 'Changed' }]\n")
	closeErr := f.Close()
	if err != nil {
		return "", false, err
	}
	if closeErr != nil {
		return "", false, closeErr
	}
	version, err := cliff("--config", f.Name(), "--bumped-version", "--bump", level)
	if err != nil {
		return "", false, err
	}
	if version != expected {
		return "", false, fmt.Errorf("git-cliff bump disagrees with stable release policy")
	}
	return version, true, nil
}
func ValidateMetadata(source, final, version string) error {
	parent, err := git("show", "-s", "--format=%P", final)
	if err != nil {
		return err
	}
	if parent != source {
		return fmt.Errorf("release metadata must have exactly the qualified source parent")
	}
	names, err := workflow.Git("diff", "--name-only", "-z", source, final, "--")
	if err != nil {
		return err
	}
	seen := map[string]bool{}
	for _, name := range workflow.SplitPaths(names) {
		if name != "CHANGELOG.md" && name != ".github/atomic-release.json" {
			return fmt.Errorf("release commit changes source: %s", name)
		}
		mode, err := git("ls-tree", final, "--", name)
		if err != nil {
			return err
		}
		if !strings.HasPrefix(mode, "100644 blob ") {
			return fmt.Errorf("metadata must be regular files")
		}
		seen[name] = true
	}
	if !seen["CHANGELOG.md"] || !seen[".github/atomic-release.json"] {
		return fmt.Errorf("release evidence and notes both required")
	}
	notes, err := git("show", final+":CHANGELOG.md")
	if err != nil {
		return err
	}
	return ValidateNotes(notes, version)
}
func ValidateNotes(notes, version string) error {
	if !strings.Contains(notes, "## [Unreleased]") || !strings.Contains(notes, "## ["+version+"] - ") ||
		!strings.Contains(notes, "["+version+"]: https://github.com/") {
		return fmt.Errorf("dated Keep a Changelog notes, Unreleased and comparison link required")
	}
	return nil
}
func ReadTag(version, repository string) (Evidence, string, error) {
	var e Evidence
	if !Stable.MatchString(version) {
		return e, "", fmt.Errorf("stable no-v tag required")
	}
	typ, err := git("cat-file", "-t", "refs/tags/"+version)
	if err != nil || typ != "tag" {
		return e, "", fmt.Errorf("annotated release tag required")
	}
	final, err := git("rev-parse", "refs/tags/"+version+"^{commit}")
	if err != nil {
		return e, "", err
	}
	raw, err := git("show", final+":.github/atomic-release.json")
	if err != nil {
		return e, "", err
	}
	if err = json.Unmarshal([]byte(raw), &e); err != nil {
		return e, "", err
	}
	if err = ValidateEvidence(e, version, repository); err != nil {
		return e, "", err
	}
	if err = ValidateMetadata(e.SourceSHA, final, version); err != nil {
		return e, "", err
	}
	head, err := git("rev-parse", "HEAD")
	if err != nil {
		return e, "", err
	}
	if head != final {
		return e, "", fmt.Errorf("checkout is not tagged release metadata")
	}
	return e, final, nil
}

func authenticatedGit(token string, args ...string) (string, error) {
	dir, err := os.MkdirTemp("", "atomic-auth-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(dir)
	ask := filepath.Join(dir, "askpass")
	if err = os.WriteFile(ask, []byte("#!/bin/sh\ncase \"$1\" in *Username*) printf '%s\\n' x-access-token ;; *) printf '%s\\n' \"$ATOMIC_RELEASE_TOKEN\" ;; esac\n"), 0700); err != nil {
		return "", err
	}
	root, err := os.Getwd()
	if err != nil {
		return "", err
	}
	base := []string{"-c", "safe.directory=" + root, "-c", "credential.helper=", "-c", "http.extraHeader=", "-c", "http.https://github.com/.extraheader=", "-c", "core.askPass=" + ask}
	cmd := exec.Command("git", append(base, args...)...)
	cmd.Env = append(os.Environ(), "GIT_ASKPASS="+ask, "GIT_TERMINAL_PROMPT=0", "ATOMIC_RELEASE_TOKEN="+token)
	data, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("authenticated Git operation failed")
	}
	return strings.TrimSpace(string(data)), nil
}
