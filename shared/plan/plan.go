// Package plan discovers action dependencies and selects work from changed files.
// The execution plan is an output artifact, never a hand-maintained catalogue.
package plan

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"go.yaml.in/yaml/v3"
)

type Component struct {
	Directory string
	Image     string
	Reference string
	Sources   []string
	Uses      []string
}

type Variant struct {
	Image     string `json:"image"`
	Directory string `json:"directory"`
	Arch      string `json:"arch"`
	Runner    string `json:"runner"`
	Rebuild   bool   `json:"rebuild"`
}

type Matrix struct {
	Include []Variant `json:"include"`
}

type Plan struct {
	Matrix             Matrix              `json:"matrix"`
	Checks             map[string][]string `json:"checks"`
	Actions            []string            `json:"actions"`
	GoPackages         []string            `json:"go-packages"`
	OrchestrationTests bool                `json:"orchestration-tests"`
	Deleted            []string            `json:"deleted"`
}

type Metadata struct {
	Runs struct {
		Using string `yaml:"using"`
		Image string `yaml:"image"`
		Steps []struct {
			Uses string            `yaml:"uses"`
			Env  map[string]string `yaml:"env"`
		} `yaml:"steps"`
	} `yaml:"runs"`
}

// copySources reads repository-root COPY/ADD dependencies. Stage copies are
// excluded; JSON arrays, continuations and flags are supported. Unresolvable
// variables select the whole context rather than silently missing dependencies.
func copySources(data string) ([]string, error) {
	var sources []string
	scanner := bufio.NewScanner(strings.NewReader(strings.ReplaceAll(data, "\\\n", " ")))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		fields := strings.Fields(line)
		if len(fields) < 2 || (strings.ToUpper(fields[0]) != "COPY" && strings.ToUpper(fields[0]) != "ADD") {
			continue
		}
		line = strings.TrimSpace(line[len(fields[0]):])
		fromStage := false
		for strings.HasPrefix(line, "--") {
			flag, rest, ok := strings.Cut(line, " ")
			if !ok {
				return nil, fmt.Errorf("invalid copy instruction: %s", line)
			}
			if strings.HasPrefix(flag, "--from=") {
				fromStage = true
			}
			line = strings.TrimSpace(rest)
		}
		if fromStage {
			continue
		}
		var paths []string
		if strings.HasPrefix(line, "[") {
			if err := json.Unmarshal([]byte(line), &paths); err != nil {
				return nil, err
			}
		} else {
			paths = strings.Fields(line)
		}
		if len(paths) < 2 {
			return nil, fmt.Errorf("copy requires source and destination: %s", line)
		}
		for _, source := range paths[:len(paths)-1] {
			if strings.Contains(source, "://") {
				continue
			}
			if strings.Contains(source, "$") {
				source = "."
			}
			sources = append(sources, filepath.ToSlash(filepath.Clean(source)))
		}
	}
	return sources, scanner.Err()
}

func Discover(root string) ([]Component, error) {
	var components []Component
	paths, err := filepath.Glob(filepath.Join(root, "*", "*", "action.yml"))
	if err != nil {
		return nil, err
	}
	for _, path := range paths {
		relative, _ := filepath.Rel(root, filepath.Dir(path))
		if strings.HasPrefix(relative, ".") || strings.HasPrefix(relative, "tests/") {
			continue
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		var metadata Metadata
		if err = yaml.Unmarshal(data, &metadata); err != nil {
			return nil, fmt.Errorf("%s: %w", relative, err)
		}
		component := Component{Directory: filepath.ToSlash(relative), Reference: metadata.Runs.Image}
		for _, step := range metadata.Runs.Steps {
			if strings.HasPrefix(step.Uses, "./") {
				component.Uses = append(component.Uses, strings.TrimPrefix(step.Uses, "./"))
			}
			if reference := step.Env["BUILDER_IMAGE"]; reference != "" {
				component.Reference = reference
			}
		}
		containerfile, err := os.ReadFile(filepath.Join(filepath.Dir(path), "Containerfile"))
		if os.IsNotExist(err) && metadata.Runs.Using == "composite" {
			components = append(components, component)
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("%s: %w", relative, err)
		}
		component.Image = strings.ReplaceAll(component.Directory, "/", "-")
		reference := strings.TrimPrefix(component.Reference, "docker://")
		image := strings.Split(strings.Split(reference[strings.LastIndex(reference, "/")+1:], "@")[0], ":")[0]
		if image != component.Image {
			return nil, fmt.Errorf("%s: image must be %s", relative, component.Image)
		}
		component.Sources, err = copySources(string(containerfile))
		if err != nil {
			return nil, fmt.Errorf("%s: %w", relative, err)
		}
		components = append(components, component)
	}
	return components, nil
}

func dependency(source, changed string) bool {
	if source == "." || source == changed || strings.HasPrefix(changed, source+"/") {
		return true
	}
	// Docker wildcards may select an entire directory whose descendants changed.
	for candidate := changed; candidate != "."; candidate = filepath.Dir(candidate) {
		if matched, _ := filepath.Match(source, candidate); matched {
			return true
		}
	}
	return false
}

func classify(root, path string) []string {
	name := filepath.Base(path)
	ext := strings.ToLower(filepath.Ext(path))
	var kinds []string
	if strings.HasPrefix(path, ".github/workflows/") && (ext == ".yml" || ext == ".yaml") {
		kinds = append(kinds, "github-workflow")
	}
	if name == "Containerfile" || name == "Dockerfile" || strings.HasPrefix(name, "Containerfile.") || strings.HasPrefix(name, "Dockerfile.") {
		kinds = append(kinds, "containerfile")
	}
	switch ext {
	case ".md", ".markdown":
		kinds = append(kinds, "markdown", "links")
	case ".yml", ".yaml":
		kinds = append(kinds, "yaml")
	case ".toml":
		kinds = append(kinds, "toml")
	case ".sh", ".bash", ".ksh":
		kinds = append(kinds, "shell")
	default:
		f, err := os.Open(filepath.Join(root, path))
		if err == nil {
			reader := bufio.NewReader(f)
			line, _, _ := reader.ReadLine()
			f.Close()
			header := string(line)
			if strings.HasPrefix(header, "#!") && (strings.Contains(header, "/sh") || strings.Contains(header, "/bash") || strings.Contains(header, "/dash") || strings.Contains(header, "env sh") || strings.Contains(header, "env bash")) {
				kinds = append(kinds, "shell")
			}
		}
	}
	if name == "action.yml" {
		parts := strings.Split(path, "/")
		if len(parts) == 3 {
			switch parts[0] {
			case "oci", "check", "workflow", "maintenance":
				kinds = append(kinds, "action-metadata")
			}
		}
	}
	return kinds
}

func Select(root string, paths []string, full bool) (Plan, error) {
	components, err := Discover(root)
	if err != nil {
		return Plan{}, err
	}
	result := Plan{Matrix: Matrix{Include: []Variant{}}, Checks: map[string][]string{}, Actions: []string{}, GoPackages: []string{}, Deleted: []string{}}
	selected, rebuilt, packages := map[string]bool{}, map[string]bool{}, map[string]bool{}
	if full {
		err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			relative, _ := filepath.Rel(root, path)
			if entry.IsDir() {
				if entry.Name() == ".git" || entry.Name() == ".build" || entry.Name() == ".candidate-actions" {
					return filepath.SkipDir
				}
				return nil
			}
			if entry.Type().IsRegular() {
				paths = append(paths, filepath.ToSlash(relative))
			}
			return nil
		})
		if err != nil {
			return Plan{}, err
		}
		for _, component := range components {
			selected[component.Directory], rebuilt[component.Directory] = true, component.Image != ""
		}
	}
	seen := map[string]bool{}
	// Configuration changes invalidate all files consumed by the corresponding
	// checker, including when that configuration has just been deleted.
	configured := map[string]bool{}
	for _, path := range paths {
		switch filepath.ToSlash(filepath.Clean(path)) {
		case ".markdownlint-cli2.jsonc", ".markdownlint.json", ".markdownlint.yaml":
			configured["markdown"] = true
		case "lychee.toml":
			configured["links"] = true
		}
	}
	if len(configured) > 0 && !full {
		if err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if entry.IsDir() {
				if entry.Name() == ".git" || entry.Name() == ".build" || entry.Name() == ".candidate-actions" {
					return filepath.SkipDir
				}
				return nil
			}
			if !entry.Type().IsRegular() {
				return nil
			}
			relative, _ := filepath.Rel(root, path)
			relative = filepath.ToSlash(relative)
			for _, kind := range classify(root, relative) {
				if configured[kind] {
					result.Checks[kind] = append(result.Checks[kind], relative)
				}
			}
			return nil
		}); err != nil {
			return Plan{}, err
		}
	}
	for _, path := range paths {
		path = filepath.ToSlash(filepath.Clean(path))
		if path == ".." || strings.HasPrefix(path, "../") || filepath.IsAbs(path) {
			return Plan{}, fmt.Errorf("path outside repository: %s", path)
		}
		if seen[path] {
			continue
		}
		seen[path] = true
		info, statErr := os.Stat(filepath.Join(root, path))
		if os.IsNotExist(statErr) {
			result.Deleted = append(result.Deleted, path)
		} else if statErr != nil {
			return Plan{}, statErr
		} else if info.Mode().IsRegular() {
			for _, kind := range classify(root, path) {
				if configured[kind] && !full {
					continue
				}
				result.Checks[kind] = append(result.Checks[kind], path)
			}
		}
		if strings.HasSuffix(path, ".go") {
			directory := filepath.Dir(path)
			if matches, _ := filepath.Glob(filepath.Join(root, directory, "*.go")); len(matches) != 0 {
				packages["./"+filepath.ToSlash(directory)] = true
			}
		}
		if path == "go.mod" || path == "go.sum" {
			packages["./..."] = true
		}
		if strings.HasPrefix(path, "scripts/") || strings.HasPrefix(path, "tests/") || strings.HasPrefix(path, ".github/workflows/") {
			result.OrchestrationTests = true
		}
		for _, component := range components {
			directory := component.Directory
			if path == directory+"/action.yml" || path == directory+"/run.py" {
				selected[directory] = true
			}
			if component.Image == "" {
				continue
			}
			if path == directory+"/Containerfile" || path == directory+"/.containerignore" || path == ".containerignore" || path == ".dockerignore" {
				rebuilt[directory] = true
			}
			for _, source := range component.Sources {
				if dependency(source, path) {
					rebuilt[directory] = true
				}
			}
			if path == "grype-review.json" {
				selected[directory] = true
			}
			if rebuilt[directory] {
				selected[directory] = true
			}
		}
	}
	// Propagate action interface changes through composite dependencies to a fixed point.
	for changed := true; changed; {
		changed = false
		for _, component := range components {
			for _, used := range component.Uses {
				if selected[used] && !selected[component.Directory] {
					selected[component.Directory], changed = true, true
				}
			}
		}
	}
	for _, component := range components {
		if !selected[component.Directory] {
			continue
		}
		result.Actions = append(result.Actions, component.Directory)
		if component.Image == "" {
			continue
		}
		if matches, _ := filepath.Glob(filepath.Join(root, component.Directory, "*.go")); len(matches) != 0 {
			packages["./"+component.Directory] = true
		}
		for _, arch := range []string{"amd64", "arm64"} {
			runner := "ubuntu-24.04"
			if arch == "arm64" {
				runner += "-arm"
			}
			result.Matrix.Include = append(result.Matrix.Include, Variant{component.Image, component.Directory, arch, runner, rebuilt[component.Directory]})
		}
	}
	if len(result.Matrix.Include) > 256 {
		return Plan{}, fmt.Errorf("matrix exceeds GitHub's 256-job limit")
	}
	for value := range packages {
		result.GoPackages = append(result.GoPackages, value)
	}
	if packages["./..."] {
		result.GoPackages = []string{"./..."}
	}
	for _, values := range result.Checks {
		sort.Strings(values)
	}
	sort.Strings(result.Actions)
	sort.Strings(result.GoPackages)
	sort.Strings(result.Deleted)
	return result, nil
}
