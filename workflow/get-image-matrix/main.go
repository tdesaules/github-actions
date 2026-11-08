package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/tdesaules/github-actions/shared/catalog"
	"github.com/tdesaules/github-actions/shared/workflow"
)

var semver = regexp.MustCompile(`^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)(?:-((?:0|[1-9]\d*|\d*[A-Za-z-][0-9A-Za-z-]*)(?:\.(?:0|[1-9]\d*|\d*[A-Za-z-][0-9A-Za-z-]*))*))?(?:\+([0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*))?$`)

type component struct{ directory, image, reference string }
type job struct {
	Image          string `json:"image"`
	Directory      string `json:"directory"`
	Arch           string `json:"arch"`
	Runner         string `json:"runner"`
	Rebuild        bool   `json:"rebuild"`
	Reference      string `json:"reference"`
	Scan           bool   `json:"scan"`
	Dependency     bool   `json:"dependency,omitempty"`
	ArtifactID     string `json:"artifact-id,omitempty"`
	SourceRun      string `json:"source-run,omitempty"`
	SourceRevision string `json:"source-revision,omitempty"`
	SourceVersion  string `json:"source-version,omitempty"`
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
			image, ref, err := catalog.Reference(data, dir)
			if err != nil {
				return nil, err
			}
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

// Development history affects only selected images; release preparation opts
// into allTests. A missing release baseline conservatively rebuilds that selection.
type rebuildHistory struct {
	paths    []string
	all      bool
	base     string
	before   string
	allTests bool
}

func sourcePath(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	return ext != ".md" && ext != ".markdown" && filepath.Base(path) != "action.yml"
}

func unreleasedChanges(head string) (rebuildHistory, error) {
	shallow, err := workflow.Git("rev-parse", "--is-shallow-repository")
	if err != nil {
		return rebuildHistory{}, err
	}
	if strings.TrimSpace(shallow) != "false" {
		return rebuildHistory{}, fmt.Errorf("release-baseline selection requires complete Git history (fetch-depth: 0)")
	}
	tags, err := workflow.Git("tag", "--merged", head, "--sort=-version:refname")
	if err != nil {
		return rebuildHistory{}, err
	}
	for _, tag := range strings.Fields(tags) {
		if !semver.MatchString(tag) || strings.ContainsAny(tag, "-+") {
			continue
		}
		base, err := workflow.Git("rev-parse", "--verify", "--end-of-options", "refs/tags/"+tag+"^{commit}")
		if err != nil {
			return rebuildHistory{}, err
		}
		paths, err := workflow.Paths(strings.TrimSpace(base), head)
		return rebuildHistory{paths: paths, base: strings.TrimSpace(base)}, err
	}
	return rebuildHistory{all: true}, nil
}

func selectImages(components []component, paths []string, full bool, histories ...rebuildHistory) (matrix, error) {
	r := matrix{Include: []job{}}
	before := ""
	if len(histories) > 0 {
		before = histories[0].before
	}
	qualification, err := qualificationChanges(components, paths, before)
	if err != nil {
		return r, err
	}
	ignoreChanges := make([]*ignoreChange, len(histories))
	for i, history := range histories {
		if history.base != "" && (hasIgnorePath(paths) || hasIgnorePath(history.paths)) {
			var err error
			ignoreChanges[i], err = loadIgnoreChange(history.base, history.paths)
			if err != nil {
				return r, err
			}
		}
	}
	for _, c := range components {
		dependencies, err := buildDependencies(c.directory)
		if err != nil {
			return r, err
		}
		selected, rebuild := full || qualification[c.image], full
		scan := full
		for _, history := range histories {
			selected = selected || history.allTests
			scan = scan || history.allTests
		}
		for _, p := range paths {
			ext := strings.ToLower(filepath.Ext(p))
			if ext == ".md" || ext == ".markdown" {
				continue
			}
			if p == "grype-review.json" || strings.HasPrefix(p, "shared/review/") {
				selected, scan = true, true
			}
			if p == c.directory+"/Containerfile" {
				selected, rebuild = true, true
				continue
			}
			if isIgnorePath(p) {
				impact := len(histories) == 0
				for i := range histories {
					affected, err := ignoreChanges[i].affects(dependencies)
					if err != nil {
						return r, err
					}
					impact = impact || affected
				}
				rebuild = rebuild || impact
				selected = selected || impact
				continue
			}
			affected, err := dependencies.affected(p)
			if err != nil {
				return r, err
			}
			if affected && sourcePath(p) {
				selected, rebuild = true, true
				continue
			}
			if strings.HasPrefix(p, c.directory+"/") {
				selected = true
			}
		}
		if !selected {
			continue
		}
		for i, history := range histories {
			rebuild = rebuild || history.all
			for _, p := range history.paths {
				if !sourcePath(p) {
					continue
				}
				if isIgnorePath(p) {
					affected, err := ignoreChanges[i].affects(dependencies)
					if err != nil {
						return r, err
					}
					rebuild = rebuild || affected
					continue
				}
				affected, err := dependencies.affected(p)
				if err != nil {
					return r, err
				}
				if affected || p == c.directory+"/Containerfile" {
					rebuild = true
				}
			}
		}
		rebuild = rebuild || c.reference == "" || strings.HasSuffix(c.reference, ":unpublished")
		scan = scan || rebuild
		for _, arch := range []string{"amd64", "arm64"} {
			runner := "ubuntu-24.04"
			if arch == "arm64" {
				runner += "-arm"
			}
			r.Include = append(r.Include, job{Image: c.image, Directory: c.directory, Arch: arch, Runner: runner, Rebuild: rebuild, Reference: c.reference, Scan: scan})
		}
	}
	return r, nil
}

// buildMatrix retains product builds and adds at most one advisory reviewer per
// scanning architecture. Published-image tests never allocate empty build jobs.
func buildMatrix(tests matrix) matrix {
	builds := matrix{Include: []job{}}
	reviewers := map[string]bool{}
	for _, j := range tests.Include {
		if j.Rebuild && j.ArtifactID == "" {
			builds.Include = append(builds.Include, j)
			if j.Image == "workflow-new-vulnerability-review" {
				reviewers[j.Arch] = true
			}
		}
	}
	for _, j := range tests.Include {
		if (j.Scan || j.Rebuild) && !reviewers[j.Arch] {
			builds.Include = append(builds.Include, job{
				Image: "workflow-new-vulnerability-review", Directory: "workflow/new-vulnerability-review",
				Arch: j.Arch, Runner: j.Runner, Rebuild: true, Dependency: true,
			})
			reviewers[j.Arch] = true
		}
	}
	return builds
}

func run() error {
	files := flag.String("files", workflow.Default("INPUT_FILES", "[]"), "changed paths JSON")
	full := flag.Bool("full", os.Getenv("INPUT_FULL") == "true", "select all images")
	event := flag.String("event", workflow.Default("INPUT_EVENT", os.Getenv("GITHUB_EVENT_NAME")), "event name")
	ref := flag.String("ref", workflow.Default("INPUT_REF", os.Getenv("GITHUB_REF")), "Git ref")
	plan := flag.String("release-plan", os.Getenv("INPUT_RELEASE-PLAN"), "release plan JSON path")
	reuse := flag.Bool("reuse-artifacts", os.Getenv("INPUT_REUSE-ARTIFACTS") == "true", "skip builds after artifact validation")
	reuseBuilds := flag.Bool("reuse-builds", os.Getenv("MATRIX_REUSE_BUILDS") == "true", "reuse source-equivalent successful development builds")
	repository := flag.String("repository", workflow.Default("INPUT_REPOSITORY", os.Getenv("GITHUB_REPOSITORY")), "build cache repository")
	before := flag.String("before", os.Getenv("MATRIX_BEFORE"), "previous push commit for qualification changes")
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
	qualifyAll := mode == "prepare"
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
				qualifyAll = false
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
	if mode == "release" || mode == "maintenance" {
		for _, c := range components {
			if !catalog.Pinned.MatchString(c.reference) {
				return fmt.Errorf("%s: released catalogue requires digest pins", c.directory)
			}
		}
	}
	m := matrix{Include: []job{}}
	if mode == "development" || mode == "prepare" {
		history := rebuildHistory{}
		if !*full {
			history, err = unreleasedChanges(head)
			if err != nil {
				return err
			}
		}
		history.before = *before
		history.allTests = qualifyAll
		m, err = selectImages(components, paths, *full, history)
		if err != nil {
			return err
		}
	}
	if mode == "prepare" && *reuse {
		m.Include = []job{}
	}
	if *reuseBuilds && mode == "development" && !*full {
		cache := buildCache{client: &http.Client{Timeout: 60 * time.Second, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}, endpoint: "https://api.github.com", token: os.Getenv("INPUT_TOKEN"), repository: *repository}
		if err := cache.reuse(&m, head, strings.TrimPrefix(*ref, "refs/heads/")); err != nil {
			// Cache availability never authorizes stale images or skipped tests.
			// On any lookup failure retain the original build plan.
			fmt.Fprintf(os.Stderr, "build cache unavailable; rebuilding candidates: %v\n", err)
		}
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
	builds := buildMatrix(m)
	reusedBuilds, published := 0, 0
	for _, j := range m.Include {
		if j.ArtifactID != "" {
			reusedBuilds++
		}
		if !j.Rebuild {
			published++
		}
	}
	fmt.Fprintf(os.Stderr, "Image plan: %d qualification jobs, %d build jobs, %d reused build archives, %d published variants\n", len(m.Include), len(builds.Include), reusedBuilds, published)
	if len(builds.Include) > 256 {
		return fmt.Errorf("build matrix exceeds 256 jobs")
	}
	for name, value := range map[string]any{"mode": mode, "version": version, "matrix": m, "build-matrix": builds, "has-build": len(builds.Include) > 0, "has-work": len(m.Include) > 0, "test-registry-image": registry} {
		if err := workflow.Output(name, value); err != nil {
			return err
		}
	}
	return nil
}
func main() { workflow.Main("get-image-matrix 1", run) }
