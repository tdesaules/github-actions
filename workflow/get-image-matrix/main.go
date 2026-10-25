package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/tdesaules/github-actions/shared/workflow"
)

var imageLine = regexp.MustCompile(`(?m)^(?:  image|        BUILDER_IMAGE): docker://([^\s]+)$`)
var semver = regexp.MustCompile(`^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)(?:-((?:0|[1-9]\d*|\d*[A-Za-z-][0-9A-Za-z-]*)(?:\.(?:0|[1-9]\d*|\d*[A-Za-z-][0-9A-Za-z-]*))*))?(?:\+([0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*))?$`)

type component struct{ directory, image, reference string }
type job struct {
	Image     string `json:"image"`
	Directory string `json:"directory"`
	Arch      string `json:"arch"`
	Runner    string `json:"runner"`
	Rebuild   bool   `json:"rebuild"`
	Reference string `json:"reference"`
}
type matrix struct {
	Include []job `json:"include"`
}

func discover() ([]component, error) {
	result := []component{}
	owners := map[string]bool{}
	for _, family := range []string{"oci", "check", "lint", "workflow", "maintenance"} {
		paths, err := filepath.Glob(family + "/*/action.yml")
		if err != nil {
			return nil, err
		}
		for _, path := range paths {
			dir := filepath.Dir(path)
			_, err := os.Stat(dir + "/Containerfile")
			if family == "workflow" && os.IsNotExist(err) {
				continue
			}
			if err != nil {
				return nil, err
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return nil, err
			}
			match := imageLine.FindSubmatch(data)
			if match == nil {
				return nil, fmt.Errorf("%s: missing image", path)
			}
			ref := string(match[1])
			image := strings.Split(strings.Split(filepath.Base(ref), ":")[0], "@")[0]
			if owners[image] {
				return nil, fmt.Errorf("duplicate image owner: %s", image)
			}
			owners[image] = true
			if family != "lint" && image != family+"-"+filepath.Base(dir) {
				return nil, fmt.Errorf("%s: incorrect image owner", path)
			}
			result = append(result, component{dir, image, ref})
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].image < result[j].image })
	return result, nil
}

func selectImages(components []component, paths []string, full bool) matrix {
	r := matrix{Include: []job{}}
	for _, c := range components {
		selected, rebuild := full, full
		for _, p := range paths {
			ext := strings.ToLower(filepath.Ext(p))
			if ext == ".md" || ext == ".markdown" {
				continue
			}
			if strings.HasPrefix(p, ".github/workflows/") || p == "grype-review.json" {
				selected = true
			}
			shared := p == "go.mod" || p == "go.sum" || p == ".containerignore" || p == ".dockerignore" || p == "taskfile.yml"
			for _, prefix := range []string{"shared/", "scripts/", "tasks/"} {
				shared = shared || strings.HasPrefix(p, prefix)
			}
			if shared {
				selected, rebuild = true, true
			}
			if strings.HasPrefix(p, c.directory+"/") {
				selected = true
				name := strings.TrimPrefix(p, c.directory+"/")
				if name == "Containerfile" || name == ".containerignore" || ext == ".go" || ext == ".mjs" || ext == ".cjs" || strings.HasPrefix(name, "src/") || strings.HasPrefix(name, "dependencies.") {
					rebuild = true
				}
			}
		}
		if !selected {
			continue
		}
		for _, arch := range []string{"amd64", "arm64"} {
			runner := "ubuntu-24.04"
			if arch == "arm64" {
				runner += "-arm"
			}
			r.Include = append(r.Include, job{Image: c.image, Directory: c.directory, Arch: arch, Runner: runner, Rebuild: rebuild || strings.HasSuffix(c.reference, ":unpublished"), Reference: c.reference})
		}
	}
	return r
}

func run() error {
	files := flag.String("files", workflow.Default("INPUT_FILES", "[]"), "changed paths JSON")
	full := flag.Bool("full", os.Getenv("INPUT_FULL") == "true", "select all images")
	event := flag.String("event", workflow.Default("INPUT_EVENT", os.Getenv("GITHUB_EVENT_NAME")), "event name")
	ref := flag.String("ref", workflow.Default("INPUT_REF", os.Getenv("GITHUB_REF")), "Git ref")
	plan := flag.String("release-plan", os.Getenv("INPUT_RELEASE-PLAN"), "release plan JSON path")
	reuse := flag.Bool("reuse-artifacts", os.Getenv("INPUT_REUSE-ARTIFACTS") == "true", "skip builds after artifact validation")
	flag.Parse()
	if flag.NArg() != 0 {
		return fmt.Errorf("unexpected arguments")
	}
	var paths []string
	if err := json.Unmarshal([]byte(*files), &paths); err != nil || paths == nil {
		return fmt.Errorf("files must be a JSON array")
	}
	head, err := workflow.Git("rev-parse", "HEAD")
	if err != nil {
		return err
	}
	head = strings.TrimSpace(head)
	mode := "development"
	if *event == "workflow_dispatch" {
		if *ref != "refs/heads/main" {
			return fmt.Errorf("release preparation must target main")
		}
		mode = "prepare"
	}
	if *event == "schedule" {
		mode = "maintenance"
	}
	version := "0.0.0-sha-" + head[:12]
	if strings.HasPrefix(*ref, "refs/tags/") {
		mode = "release"
		version = strings.TrimPrefix(*ref, "refs/tags/")
		if !semver.MatchString(version) {
			return fmt.Errorf("tag must be SemVer without v")
		}
	}
	if mode == "prepare" {
		*full = true
		if *plan != "" {
			data, err := os.ReadFile(*plan)
			if err != nil {
				return err
			}
			var p struct {
				Base     string
				Previous string
				Changed  bool
			}
			if err := json.Unmarshal(data, &p); err != nil {
				return err
			}
			if p.Base != head {
				return fmt.Errorf("release plan does not match checkout")
			}
			if !p.Changed {
				*full = false
				paths = []string{}
			} else if p.Previous != "" {
				*full = false
				base, resolveErr := workflow.Git("rev-parse", "--verify", "--end-of-options", p.Previous+"^{commit}")
				if resolveErr != nil {
					return resolveErr
				}
				paths, err = workflow.Paths(strings.TrimSpace(base), head)
				if err != nil {
					return err
				}
			}
		}
	}
	components, err := discover()
	if err != nil {
		return err
	}
	m := matrix{Include: []job{}}
	if mode == "development" || mode == "prepare" {
		m = selectImages(components, paths, *full)
	}
	if mode == "prepare" && *reuse {
		m.Include = []job{}
	}
	if len(m.Include) > 256 {
		return fmt.Errorf("matrix exceeds 256 jobs")
	}
	registry := ""
	for _, c := range components {
		if c.directory == "workflow/start-test-registry" {
			registry = c.reference
		}
	}
	for name, value := range map[string]any{"mode": mode, "version": version, "matrix": m, "has-work": len(m.Include) > 0, "test-registry-image": registry} {
		if err := workflow.Output(name, value); err != nil {
			return err
		}
	}
	return nil
}
func main() { workflow.Main("get-image-matrix 1", run) }
