package binaryrelease

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

type Matrix struct {
	Include []map[string]any `json:"include"`
}
type Plan struct {
	SchemaVersion int    `json:"schema_version"`
	Reason        string `json:"reason"`
	Mode          string `json:"mode"`
	SourceSHA     string `json:"source_sha"`
	Version       string `json:"version"`
	HasLint       bool   `json:"has_lint"`
	HasImages     bool   `json:"has_images"`
	HasTests      bool   `json:"has_tests"`
	HasRelease    bool   `json:"has_release"`
	LintMatrix    Matrix `json:"lint_matrix"`
	BuildMatrix   Matrix `json:"build_matrix"`
	TestMatrix    Matrix `json:"test_matrix"`
	PlanArtifact  string `json:"plan_artifact"`
}
type PlanOptions struct {
	Event, Ref, Before, SourceSHA, Mode, Config, Output string
	Initial                                             bool
}

func matrix() Matrix { return Matrix{Include: []map[string]any{}} }
func platforms() Matrix {
	return Matrix{Include: []map[string]any{{"component": Component, "directory": ".", "arch": "amd64", "runner": "ubuntu-24.04"}, {"component": Component, "directory": ".", "arch": "arm64", "runner": "ubuntu-24.04-arm"}}}
}
func testPlatforms(source string) Matrix {
	m := platforms()
	for _, row := range m.Include {
		row["source"] = source
		row["runtime_test"] = true
		row["scan"] = true
	}
	return m
}
func latestStable() (string, error) {
	s, e := Git("tag", "--merged", "HEAD")
	if e != nil {
		return "", e
	}
	latest := ""
	for _, t := range strings.Fields(s) {
		if stable.MatchString(t) && (latest == "" || VersionCompare(t, latest) > 0) {
			latest = t
		}
	}
	return latest, nil
}

// Bump returns the policy level separately from changelog filtering.
func Bump(messages []string) string {
	level := 0
	for _, m := range messages {
		line := strings.ToLower(strings.SplitN(m, "\n", 2)[0])
		colon := strings.Index(line, ":")
		if colon < 0 {
			continue
		}
		typ := line[:colon]
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
func releaseVersion(initial bool, config string) (string, bool, error) {
	previous, e := latestStable()
	if e != nil {
		return "", false, e
	}
	if previous == "" {
		if initial {
			return "1.0.0", true, nil
		}
		return "0.1.0", false, nil
	}
	if initial {
		return "", false, fmt.Errorf("initial-release requires no prior stable tag")
	}
	raw, e := gitRaw("log", previous+"..HEAD", "--format=%B%x00")
	if e != nil {
		return "", false, e
	}
	level := Bump(strings.Split(string(raw), "\x00"))
	if level == "" {
		return previous, false, nil
	}
	if strings.HasPrefix(previous, "0.") && level == "major" {
		level = "minor"
	}
	// Preserve the application's tag/bump policy but remove note filtering only
	// for this version query. git-cliff otherwise treats an entirely hidden range
	// as empty even with an explicit --bump (e.g. uppercase FEAT/PERF).
	data, e := os.ReadFile(config)
	if e != nil {
		return "", false, e
	}
	var cfg map[string]any
	if e = toml.Unmarshal(data, &cfg); e != nil {
		return "", false, e
	}
	git, ok := cfg["git"].(map[string]any)
	if !ok {
		git = map[string]any{}
		cfg["git"] = git
	}
	git["conventional_commits"] = false
	git["filter_unconventional"] = false
	git["filter_commits"] = false
	git["commit_preprocessors"] = []any{}
	git["commit_parsers"] = []map[string]any{{"message": ".*", "group": "Changed"}}
	data, e = toml.Marshal(cfg)
	if e != nil {
		return "", false, e
	}
	tmp, e := os.CreateTemp("", "binary-release-bump-*.toml")
	if e != nil {
		return "", false, e
	}
	defer os.Remove(tmp.Name())
	_, e = tmp.Write(data)
	closeErr := tmp.Close()
	if e != nil {
		return "", false, e
	}
	if closeErr != nil {
		return "", false, closeErr
	}
	// Explicit bump avoids git-cliff's default perf/pre-1.0 policy.
	b, e := runCliff("--offline", "--no-exec", "--config", tmp.Name(), "--bumped-version", "--bump", level)
	if e != nil {
		return "", false, fmt.Errorf("git-cliff version calculation failed")
	}
	v := strings.TrimSpace(string(b))
	if !stable.MatchString(v) || VersionCompare(v, previous) <= 0 {
		return "", false, fmt.Errorf("git-cliff returned invalid release version")
	}
	return v, true, nil
}
func changed(before, head string) ([]string, error) {
	args := []string{"diff", "--name-only", "-z", "--no-renames", before, head, "--"}
	if before == strings.Repeat("0", 40) {
		args = []string{"ls-tree", "-r", "--name-only", "-z", head}
	} else if !sha.MatchString(before) {
		return nil, fmt.Errorf("push before must be a commit SHA or explicit zero new-branch SHA")
	} else if _, e := Git("cat-file", "-e", before+"^{commit}"); e != nil {
		return nil, fmt.Errorf("push before commit is unavailable; fetch required history before planning")
	}
	b, e := gitRaw(args...)
	if e != nil {
		return nil, e
	}
	return strings.Split(strings.TrimSuffix(string(b), "\x00"), "\x00"), nil
}
func MakePlan(o PlanOptions) (Plan, error) {
	p := Plan{SchemaVersion: 1, Reason: "changed application files", Mode: "development", SourceSHA: o.SourceSHA, Version: "0.1.0", LintMatrix: matrix(), BuildMatrix: matrix(), TestMatrix: matrix(), PlanArtifact: "binary-plan-" + os.Getenv("GITHUB_RUN_ID")}
	if e := Head(o.SourceSHA); e != nil {
		return p, e
	}
	if o.Event == "workflow_dispatch" && o.Mode == "maintenance" && o.Ref != "refs/heads/main" {
		return p, fmt.Errorf("maintenance dispatch requires main")
	}
	if o.Event == "schedule" || (o.Event == "workflow_dispatch" && o.Mode == "maintenance") {
		p.Mode = "maintenance"
		p.Reason = "maintenance is independently routed"
		return p, nil
	}
	switch o.Event {
	case "workflow_dispatch":
		if o.Ref != "refs/heads/main" || o.Mode != "prepare" {
			return p, fmt.Errorf("prepare dispatch requires main and mode prepare")
		}
		p.Mode = "prepare"
	case "push":
		if strings.HasPrefix(o.Ref, "refs/tags/") {
			tag := strings.TrimPrefix(o.Ref, "refs/tags/")
			if !stable.MatchString(tag) {
				return p, fmt.Errorf("only stable no-v release tags are accepted")
			}
			m, final, e := ReadTag(tag)
			if e != nil {
				return p, e
			}
			if final != o.SourceSHA {
				return p, fmt.Errorf("tag does not identify checkout HEAD")
			}
			p.Mode = "publish"
			p.Reason = "verify complete immutable tagged binary set"
			p.SourceSHA = m.SourceSHA
			p.Version = m.Version
			p.LintMatrix, e = selectLints(m.SourceSHA, nil, true)
			if e != nil {
				return p, e
			}
			p.HasLint = len(p.LintMatrix.Include) > 0
			p.HasTests = true
			p.HasRelease = true
			p.TestMatrix = testPlatforms("published")
			return p, nil
		}
		if !strings.HasPrefix(o.Ref, "refs/heads/") || len(o.Ref) == len("refs/heads/") {
			return p, fmt.Errorf("invalid branch ref")
		}
	default:
		return p, fmt.Errorf("unsupported event")
	}
	if p.Mode == "development" {
		manifest, err := os.ReadFile("Cargo.toml")
		if err != nil {
			return p, err
		}
		p.Version, err = packageVersion(manifest)
		if err != nil {
			return p, err
		}
	}
	var paths []string
	var e error
	if p.Mode == "development" {
		paths, e = changed(o.Before, o.SourceSHA)
		if e != nil {
			return p, e
		}
	}
	build := false
	for _, path := range paths {
		if path == "" {
			continue
		}
		if runtimePath(path) || path == o.Config || path == "cliff.toml" {
			build = true
		}
	}
	if p.Mode == "prepare" {
		var release bool
		p.Version, release, e = releaseVersion(o.Initial, o.Config)
		if e != nil {
			return p, e
		}
		p.HasRelease = release
		p.Reason = "complete release qualification"
		if !release {
			p.Reason = "no release bump; initial 1.0.0 requires explicit initial-release"
		}
		build = release
		if !release {
			return p, nil
		}
	}
	p.LintMatrix, e = selectLints(o.SourceSHA, paths, p.Mode == "prepare")
	if e != nil {
		return p, e
	}
	p.HasLint = len(p.LintMatrix.Include) > 0
	p.HasImages = build
	p.HasTests = build
	if build {
		p.BuildMatrix = platforms()
		p.TestMatrix = testPlatforms("candidate")
	}
	return p, nil
}
func PlanAction() error {
	initial := Input("initial-release", "false")
	if initial != "true" && initial != "false" {
		return fmt.Errorf("initial-release must be boolean")
	}
	out := Input("output", ".build/plan.json")
	if out == "" {
		return fmt.Errorf("output path required")
	}
	workspace, e := os.Getwd()
	if e != nil {
		return e
	}
	out, e = filepath.Abs(out)
	if e != nil {
		return e
	}
	directory := Input("directory", ".")
	if directory == "" {
		return fmt.Errorf("application directory required")
	}
	p, e := func() (plan Plan, err error) {
		if err = os.Chdir(directory); err != nil {
			return plan, fmt.Errorf("cannot enter application directory")
		}
		defer func() {
			if restore := os.Chdir(workspace); restore != nil {
				err = fmt.Errorf("cannot restore workspace directory")
			}
		}()
		return MakePlan(PlanOptions{Input("event", os.Getenv("GITHUB_EVENT_NAME")), Input("ref", os.Getenv("GITHUB_REF")), Input("before", ""), Input("source-sha", os.Getenv("GITHUB_SHA")), Input("mode", ""), Input("config", "cliff.toml"), out, initial == "true"})
	}()
	if e != nil {
		return e
	}
	if e = WriteJSON(out, p); e != nil {
		return e
	}
	for k, v := range map[string]any{"mode": p.Mode, "source_sha": p.SourceSHA, "version": p.Version, "has_lint": p.HasLint, "has_images": p.HasImages, "has_tests": p.HasTests, "has_release": p.HasRelease, "lint_matrix": p.LintMatrix, "build_matrix": p.BuildMatrix, "test_matrix": p.TestMatrix, "plan_artifact": p.PlanArtifact} {
		if e = Output(k, v); e != nil {
			return e
		}
	}
	return nil
}
