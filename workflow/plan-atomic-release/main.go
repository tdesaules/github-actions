package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/tdesaules/github-actions/shared/atomicrelease"
	"github.com/tdesaules/github-actions/shared/workflow"
)

type matrix struct {
	Include []map[string]any `json:"include"`
}

type plan struct {
	SchemaVersion  int              `json:"schema_version"`
	Mode           string           `json:"mode"`
	SourceSHA      string           `json:"source_sha"`
	Version        string           `json:"version"`
	Files          []string         `json:"files"`
	HasLint        bool             `json:"has_lint"`
	HasImages      bool             `json:"has_images"`
	HasTests       bool             `json:"has_tests"`
	HasRelease     bool             `json:"has_release"`
	Lint           matrix           `json:"lint_matrix"`
	Build          matrix           `json:"build_matrix"`
	Test           matrix           `json:"test_matrix"`
	Published      *releaseEvidence `json:"published,omitempty"`
	InitialRelease bool             `json:"initial_release"`
}

var stableVersion = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$`)

type releaseEvidence = atomicrelease.Evidence

func validateEvidence(e releaseEvidence, version, repository string) error {
	return atomicrelease.ValidateEvidence(e, version, repository)
}

func mode(event, ref string) (string, error) {
	switch {
	case event == "workflow_dispatch" && ref == "refs/heads/main":
		return "prepare", nil
	case event == "push" && strings.HasPrefix(ref, "refs/tags/"):
		if !stableVersion.MatchString(strings.TrimPrefix(ref, "refs/tags/")) {
			return "", fmt.Errorf("expected a stable SemVer tag without v")
		}
		return "publish", nil
	case event == "push" && strings.HasPrefix(ref, "refs/heads/") && len(ref) > len("refs/heads/"):
		return "development", nil
	default:
		return "", fmt.Errorf("unsupported event/ref; manual release requires main")
	}
}

func maintenanceMode(event, ref string) error {
	if ref != "refs/heads/main" || (event != "schedule" && event != "workflow_dispatch") {
		return fmt.Errorf("maintenance requires schedule or manual dispatch on main")
	}
	return nil
}
func markdownFiles(files []string) []string {
	result := []string{}
	for _, f := range files {
		if strings.HasSuffix(strings.ToLower(f), ".md") {
			result = append(result, f)
		}
	}
	sort.Strings(result)
	return result
}

func selection(files []string, full bool) (matrix, bool) {
	groups := map[string][]string{}
	build := full
	for _, path := range files {
		if strings.HasPrefix(path, "recipes/") || strings.HasPrefix(path, "files/") ||
			path == "cosign.pub" || strings.HasPrefix(path, ".github/") {
			build = true
		}
		switch {
		case strings.HasPrefix(path, ".github/workflows/") && strings.HasSuffix(path, ".yml"):
			groups["actionlint"] = append(groups["actionlint"], path)
		case strings.HasSuffix(path, ".yaml") || strings.HasSuffix(path, ".yml"):
			groups["yaml"] = append(groups["yaml"], path)
		case strings.HasSuffix(path, ".toml"):
			groups["toml"] = append(groups["toml"], path)
		case strings.HasSuffix(path, ".md"):
			groups["markdownlint"] = append(groups["markdownlint"], path)
		case strings.HasSuffix(path, ".sh") || strings.HasPrefix(path, "files/system/usr/libexec/"):
			groups["shellcheck"] = append(groups["shellcheck"], path)
		}
	}
	result := matrix{Include: []map[string]any{}}
	keys := []string{}
	for key := range groups {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		sort.Strings(groups[key])
		result.Include = append(result.Include, map[string]any{
			"id": key, "action_path": "lint/" + key, "runner": "ubuntu-24.04", "files": groups[key],
		})
	}
	return result, build
}

func run() error {
	event := flag.String("event", "", "GitHub event")
	ref := flag.String("ref", "", "GitHub ref")
	before := flag.String("before", "", "previous source revision")
	version := flag.String("version", "0.1.0", "development candidate version")
	repository := flag.String("repository", "", "GitHub owner/repository")
	initial := flag.Bool("initial-release", false, "explicit first 1.0.0 release")
	maintenance := flag.Bool("maintenance", false, "pure main-ref validation and Markdown enumeration")
	output := flag.String("output", ".build/plan.json", "plan artifact")
	flag.Parse()
	if flag.NArg() != 0 {
		return fmt.Errorf("unexpected arguments")
	}
	selectedMode, err := mode(*event, *ref)
	if *maintenance {
		selectedMode = "maintenance"
		err = maintenanceMode(*event, *ref)
	}
	if err != nil {
		return err
	}
	head, err := workflow.Git("rev-parse", "HEAD")
	if err != nil {
		return err
	}
	head = strings.TrimSpace(head)
	if expected := os.Getenv("GITHUB_SHA"); expected != "" && expected != head {
		return fmt.Errorf("checkout differs from event source")
	}
	all, err := workflow.Git("ls-files", "-z")
	if err != nil {
		return err
	}
	files := workflow.SplitPaths(all)
	if *maintenance {
		md := markdownFiles(files)
		for name, value := range map[string]any{"mode": "maintenance", "source_sha": head, "markdown-files": md, "has_lint": "false", "has_images": "false", "has_tests": "false", "has_release": "false"} {
			if err := workflow.Output(name, value); err != nil {
				return err
			}
		}
		return nil
	}
	full := selectedMode != "development" || *before == "" || *before == strings.Repeat("0", 40)
	buildChanged := false
	if !full {
		if !regexp.MustCompile(`^[a-f0-9]{40}$`).MatchString(*before) {
			return fmt.Errorf("before must be a full source SHA")
		}
		changed, err := workflow.Paths(*before, head)
		if err != nil {
			return err
		}
		// Deletions still trigger a rebuild, but only existing files are linted.
		_, buildChanged = selection(changed, false)
		exists := map[string]bool{}
		for _, name := range files {
			exists[name] = true
		}
		files = nil
		for _, name := range changed {
			if exists[name] {
				files = append(files, name)
			}
		}
	}
	p := plan{SchemaVersion: 1, Mode: selectedMode, SourceSHA: head, Version: *version,
		Files: files, Build: matrix{Include: []map[string]any{}}, Test: matrix{Include: []map[string]any{}}}
	p.Lint, p.HasImages = selection(files, full)
	p.HasImages = p.HasImages || buildChanged
	p.HasLint = len(p.Lint.Include) > 0
	p.HasRelease = selectedMode == "publish"
	if selectedMode == "prepare" {
		p.InitialRelease = *initial
		p.Version, p.HasRelease, err = atomicrelease.ReleaseVersion(*initial)
		if err != nil {
			return err
		}
	}
	if selectedMode == "prepare" && !p.HasRelease {
		p.Lint.Include = []map[string]any{}
		p.HasLint, p.HasImages = false, false
		p.Version = ""
	}
	if selectedMode == "publish" {
		p.Version = strings.TrimPrefix(*ref, "refs/tags/")
		p.HasImages = false
		evidence, _, err := atomicrelease.ReadTag(p.Version, *repository)
		if err != nil {
			return err
		}
		p.SourceSHA, p.Published = evidence.SourceSHA, &evidence
	}
	if p.HasRelease && !stableVersion.MatchString(p.Version) {
		return fmt.Errorf("release plan requires a stable SemVer version")
	}
	for _, arch := range []string{"amd64", "arm64"} {
		runner := "ubuntu-24.04"
		if arch == "arm64" {
			runner = "ubuntu-24.04-arm"
		}
		if p.HasImages {
			p.Build.Include = append(p.Build.Include, map[string]any{
				"component": "os", "directory": ".", "arch": arch, "runner": runner,
			})
			p.Test.Include = append(p.Test.Include, map[string]any{
				"component": "os", "arch": arch, "runner": runner, "source": "candidate",
				"digest": "", "runtime_test": true, "scan": true,
			})
		} else if p.Published != nil {
			p.Test.Include = append(p.Test.Include, map[string]any{
				"component": "os", "arch": arch, "runner": runner, "source": "published",
				"digest": p.Published.Image, "runtime_test": true, "scan": true,
				"iso": p.Published.ISO, "iso_file": p.Published.ISOFile,
				"iso_sha256":         p.Published.ISOSHA256,
				"iso_files":          p.Published.ISOFiles,
				"os_archive_sha256":  p.Published.Qualified.AMD64.SHA256,
				"os_manifest_digest": p.Published.Qualified.AMD64.ManifestDigest,
			})
		}
	}
	p.HasTests = len(p.Test.Include) > 0
	data, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(*output), 0755); err != nil {
		return err
	}
	if err := os.WriteFile(*output, append(data, '\n'), 0644); err != nil {
		return err
	}
	for name, value := range map[string]any{
		"mode": p.Mode, "source_sha": p.SourceSHA, "version": p.Version,
		"has_lint": strconv.FormatBool(p.HasLint), "has_images": strconv.FormatBool(p.HasImages),
		"has_tests": strconv.FormatBool(p.HasTests), "has_release": strconv.FormatBool(p.HasRelease),
		"lint_matrix": p.Lint, "build_matrix": p.Build, "test_matrix": p.Test, "plan_artifact": "atomic-plan",
	} {
		if err := workflow.Output(name, value); err != nil {
			return err
		}
	}
	return nil
}

func main() { workflow.Main("plan-atomic-release 1", run) }
