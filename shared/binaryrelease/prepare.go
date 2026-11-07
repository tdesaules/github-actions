package binaryrelease

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

func metadataVersion(data []byte, lock bool, version string) (map[string]any, error) {
	var doc map[string]any
	if e := toml.Unmarshal(data, &doc); e != nil {
		return nil, e
	}
	if !lock {
		p, ok := doc["package"].(map[string]any)
		if !ok || p["name"] != Component || p["version"] != version {
			return nil, fmt.Errorf("Cargo package version mismatch")
		}
		p["version"] = "<release>"
		return doc, nil
	}
	packages, ok := doc["package"].([]any)
	if !ok {
		return nil, fmt.Errorf("invalid Cargo lock")
	}
	count := 0
	for _, p := range packages {
		pkg, ok := p.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("invalid lock package")
		}
		if pkg["name"] == Component {
			if pkg["version"] != version {
				return nil, fmt.Errorf("lock version mismatch")
			}
			pkg["version"] = "<release>"
			count++
		}
	}
	if count != 1 {
		return nil, fmt.Errorf("expected one local lock package")
	}
	return doc, nil
}
func packageVersion(data []byte) (string, error) {
	var doc struct {
		Package struct {
			Version string `toml:"version"`
		} `toml:"package"`
	}
	if e := toml.Unmarshal(data, &doc); e != nil {
		return "", e
	}
	if !stable.MatchString(doc.Package.Version) {
		return "", fmt.Errorf("invalid Cargo version")
	}
	return doc.Package.Version, nil
}
func validateMetadata(source, final, version string) error {
	parents, e := Git("show", "-s", "--format=%P", final)
	if e != nil {
		return e
	}
	if parents != source {
		return fmt.Errorf("release metadata commit must have exactly source parent")
	}
	names, e := gitRaw("diff", "--name-only", "-z", source, final, "--")
	if e != nil {
		return e
	}
	seen := map[string]bool{}
	for _, name := range strings.Split(strings.TrimSuffix(string(names), "\x00"), "\x00") {
		if name != "Cargo.toml" && name != "Cargo.lock" && name != "CHANGELOG.md" {
			return fmt.Errorf("release commit changes application source")
		}
		seen[name] = true
		mode, err := Git("ls-tree", final, "--", name)
		if err != nil {
			return err
		}
		if !strings.HasPrefix(mode, "100644 blob ") {
			return fmt.Errorf("release metadata must be regular files")
		}
	}
	if !seen["CHANGELOG.md"] {
		return fmt.Errorf("release notes required")
	}
	old, e := gitRaw("show", source+":Cargo.toml")
	if e != nil {
		return e
	}
	previous, e := packageVersion(old)
	if e != nil {
		return e
	}
	for _, name := range []string{"Cargo.toml", "Cargo.lock"} {
		before, e := gitRaw("show", source+":"+name)
		if e != nil {
			return e
		}
		after, e := gitRaw("show", final+":"+name)
		if e != nil {
			return e
		}
		a, e := metadataVersion(before, name == "Cargo.lock", previous)
		if e != nil {
			return e
		}
		b, e := metadataVersion(after, name == "Cargo.lock", version)
		if e != nil {
			return e
		}
		if !reflect.DeepEqual(a, b) {
			return fmt.Errorf("release commit changes non-version Cargo metadata")
		}
		mode, e := Git("ls-tree", final, "--", name)
		if e != nil {
			return e
		}
		if !strings.HasPrefix(mode, "100644 blob ") {
			return fmt.Errorf("metadata must be regular files")
		}
	}
	notes, e := gitRaw("show", final+":CHANGELOG.md")
	if e != nil {
		return e
	}
	if !strings.Contains(string(notes), "## ["+version+"]") || (!strings.Contains(string(notes), "## [Unreleased]") && !strings.Contains(string(notes), "## Unreleased")) {
		return fmt.Errorf("Keep a Changelog release headings required")
	}
	return nil
}
func ReadTag(tag string) (Marker, string, error) {
	var m Marker
	if !stable.MatchString(tag) {
		return m, "", fmt.Errorf("invalid stable tag")
	}
	typ, e := Git("cat-file", "-t", "refs/tags/"+tag)
	if e != nil || typ != "tag" {
		return m, "", fmt.Errorf("annotated release tag required")
	}
	raw, e := gitRaw("cat-file", "tag", "refs/tags/"+tag)
	if e != nil {
		return m, "", e
	}
	parts := strings.SplitN(string(raw), "\n\n", 2)
	if len(parts) != 2 || len(parts[1]) > 16384 || !strings.HasPrefix(parts[1], markerPrefix) {
		return m, "", fmt.Errorf("authorized release marker required")
	}
	if e = json.Unmarshal([]byte(strings.TrimPrefix(parts[1], markerPrefix)), &m); e != nil {
		return m, "", e
	}
	if e = ValidateMarker(m); e != nil {
		return m, "", e
	}
	if m.Version != tag {
		return m, "", fmt.Errorf("tag version mismatch")
	}
	final, e := Git("rev-parse", "refs/tags/"+tag+"^{commit}")
	if e != nil {
		return m, "", e
	}
	if e = validateMetadata(m.SourceSHA, final, m.Version); e != nil {
		return m, "", e
	}
	return m, final, nil
}
func updateVersion(path, previous, version string, lock bool) error {
	b, e := os.ReadFile(path)
	if e != nil {
		return e
	}
	if _, e = metadataVersion(b, lock, previous); e != nil {
		return e
	}
	// Restrict replacement to the package's section and retain formatting/comments.
	lines := strings.Split(string(b), "\n")
	section := ""
	start := -1
	changed := 0
	replace := func(end int) error {
		if start < 0 {
			return nil
		}
		isTarget := !lock
		for i := start; i < end; i++ {
			if regexp.MustCompile(`^name\s*=\s*"niri-tonic-files"\s*$`).MatchString(strings.TrimSpace(lines[i])) {
				isTarget = true
			}
		}
		if isTarget {
			for i := start; i < end; i++ {
				if regexp.MustCompile(`^version\s*=\s*"` + regexp.QuoteMeta(previous) + `"\s*$`).MatchString(strings.TrimSpace(lines[i])) {
					lines[i] = "version = \"" + version + "\""
					changed++
				}
			}
		}
		return nil
	}
	for i, line := range lines {
		trim := strings.TrimSpace(line)
		if strings.HasPrefix(trim, "[") {
			if (lock && section == "[[package]]") || (!lock && section == "[package]") {
				_ = replace(i)
			}
			section = trim
			start = i + 1
		}
	}
	if (lock && section == "[[package]]") || (!lock && section == "[package]") {
		_ = replace(len(lines))
	}
	if changed != 1 {
		return fmt.Errorf("cannot safely update Cargo package version")
	}
	return os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0644)
}
func authenticatedGit(a API, args ...string) (string, error) {
	dir, e := os.MkdirTemp("", "binary-release-auth-")
	if e != nil {
		return "", e
	}
	defer os.RemoveAll(dir)
	ask := filepath.Join(dir, "askpass")
	if e = os.WriteFile(ask, []byte("#!/bin/sh\ncase \"$1\" in *Username*) printf '%s\\n' x-access-token ;; *) printf '%s\\n' \"$BINARY_RELEASE_TOKEN\" ;; esac\n"), 0700); e != nil {
		return "", e
	}
	cmd := gitCommand(append([]string{"-c", "credential.helper=", "-c", "http.extraHeader=", "-c", "http.https://github.com/.extraheader=", "-c", "core.askPass=" + ask}, args...)...)
	cmd.Env = append(os.Environ(), "GIT_ASKPASS="+ask, "GIT_TERMINAL_PROMPT=0", "BINARY_RELEASE_TOKEN="+a.Token)
	b, e := cmd.Output()
	if e != nil {
		return "", fmt.Errorf("authenticated git operation failed")
	}
	return strings.TrimSpace(string(b)), nil
}
func PrepareAction() error {
	source := Input("source-sha", "")
	if e := Head(source); e != nil {
		return e
	}
	if os.Getenv("GITHUB_REF") != "refs/heads/main" || os.Getenv("GITHUB_EVENT_NAME") != "workflow_dispatch" {
		return fmt.Errorf("prepare requires main dispatch")
	}
	var p Plan
	if e := Decode(Input("plan", ".build/plan.json"), &p); e != nil {
		return e
	}
	if p.SchemaVersion != 1 || p.Mode != "prepare" || p.SourceSHA != source || !p.HasRelease || !stable.MatchString(p.Version) {
		return fmt.Errorf("invalid preparation plan")
	}
	a, e := NewAPI()
	if e != nil {
		return e
	}
	m := Marker{SchemaVersion: 1, SourceSHA: source, Version: p.Version, RunID: os.Getenv("GITHUB_RUN_ID"), RunAttempt: os.Getenv("GITHUB_RUN_ATTEMPT")}
	q, e := Verify(Input("qualified-directory", ".build/qualified"), source, p.Version, m.RunID, m.RunAttempt)
	if e != nil {
		return e
	}
	if e = a.Bind(&m, q); e != nil {
		return e
	}
	// Verify uploaded immutable bytes as well as the local job transfer before tagging.
	tmp, e := os.MkdirTemp("", "binary-prepare-")
	if e != nil {
		return e
	}
	defer os.RemoveAll(tmp)
	if e = a.Fetch(m, filepath.Join(tmp, "qualified"), false); e != nil {
		return e
	}
	status, e := Git("status", "--porcelain", "--untracked-files=no")
	if e != nil {
		return e
	}
	if status != "" {
		return fmt.Errorf("tracked worktree must be clean")
	}
	remote := "https://github.com/" + a.Repository + ".git"
	recheck := func() error {
		refs, e := authenticatedGit(a, "ls-remote", remote, "refs/heads/main", "refs/tags/"+p.Version)
		if e != nil {
			return e
		}
		lines := strings.Split(refs, "\n")
		if len(lines) != 1 || lines[0] != source+"\trefs/heads/main" {
			return fmt.Errorf("main advanced or release tag exists")
		}
		return nil
	}
	if e = recheck(); e != nil {
		return e
	}
	b, e := os.ReadFile("Cargo.toml")
	if e != nil {
		return e
	}
	previous, e := packageVersion(b)
	if e != nil {
		return e
	}
	for _, f := range []string{"Cargo.toml", "Cargo.lock"} {
		if e = updateVersion(f, previous, p.Version, f == "Cargo.lock"); e != nil {
			return e
		}
	}
	cmd := exec.Command("git-cliff", "--config", Input("config", "cliff.toml"), "--tag", p.Version, "--output", "CHANGELOG.md")
	if e = cmd.Run(); e != nil {
		return fmt.Errorf("git-cliff notes generation failed")
	}
	if _, e = Git("add", "--", "Cargo.toml", "Cargo.lock", "CHANGELOG.md"); e != nil {
		return e
	}
	if _, e = Git("-c", "user.name=github-actions[bot]", "-c", "user.email=41898282+github-actions[bot]@users.noreply.github.com", "commit", "-m", "chore(release): prepare "+p.Version, "-m", "Align Cargo metadata and release notes with the exact source-bound, functionally qualified Linux binaries."); e != nil {
		return e
	}
	final, e := Git("rev-parse", "HEAD")
	if e != nil {
		return e
	}
	if e = validateMetadata(source, final, p.Version); e != nil {
		return e
	}
	if e = recheck(); e != nil {
		return e
	}
	data, _ := json.Marshal(m)
	if _, e = Git("-c", "user.name=github-actions[bot]", "-c", "user.email=41898282+github-actions[bot]@users.noreply.github.com", "tag", "-a", p.Version, "-m", markerPrefix+string(data), final); e != nil {
		return e
	}
	if _, e = authenticatedGit(a, "push", "--atomic", "--force-with-lease=refs/heads/main:"+source, remote, final+":refs/heads/main", "refs/tags/"+p.Version+":refs/tags/"+p.Version); e != nil {
		return e
	}
	for k, v := range map[string]string{"tag": p.Version, "final_sha": final, "source_sha": source, "artifact_run_id": m.RunID} {
		if e = Output(k, v); e != nil {
			return e
		}
	}
	return nil
}
func ReadAction() error {
	tag := Input("tag", strings.TrimPrefix(os.Getenv("GITHUB_REF"), "refs/tags/"))
	m, final, e := ReadTag(tag)
	if e != nil {
		return e
	}
	if e = Head(final); e != nil {
		return e
	}
	a, e := NewAPI()
	if e != nil {
		return e
	}
	out := Input("output", ".build/released")
	if e = a.Fetch(m, out, true); e != nil {
		return e
	}
	for k, v := range map[string]string{"source_sha": m.SourceSHA, "version": m.Version, "run_id": m.RunID, "run_attempt": m.RunAttempt, "qualified_directory": out, "amd64_directory": filepath.Join(out, "amd64"), "arm64_directory": filepath.Join(out, "arm64")} {
		if e = Output(k, v); e != nil {
			return e
		}
	}
	return nil
}
