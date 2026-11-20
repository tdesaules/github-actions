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
	Image          string   `json:"image"`
	Directory      string   `json:"directory"`
	Arch           string   `json:"arch"`
	Runner         string   `json:"runner"`
	Rebuild        bool     `json:"rebuild"`
	Reference      string   `json:"reference"`
	Scan           bool     `json:"scan"`
	RuntimeTest    bool     `json:"runtime-test"`
	Reasons        []string `json:"reasons,omitempty"`
	Dependency     bool     `json:"dependency,omitempty"`
	ArtifactID     string   `json:"artifact-id,omitempty"`
	SourceRun      string   `json:"source-run,omitempty"`
	SourceRevision string   `json:"source-revision,omitempty"`
	SourceVersion  string   `json:"source-version,omitempty"`
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
	return ext != ".md" && ext != ".markdown"
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
		runtimeTest := full || qualification[c.image]
		reasons := []string{}
		if full {
			reasons = append(reasons, "full selection")
		}
		if qualification[c.image] {
			reasons = append(reasons, "qualification dependency changed")
		}
		scan := full
		for _, history := range histories {
			selected = selected || history.allTests
			scan = scan || history.allTests
			runtimeTest = runtimeTest || history.allTests
		}
		for _, p := range paths {
			ext := strings.ToLower(filepath.Ext(p))
			if ext == ".md" || ext == ".markdown" {
				continue
			}
			if p == "grype-review.json" || strings.HasPrefix(p, "shared/review/") {
				selected, scan = true, true
				reasons = append(reasons, "scan policy: "+p)
			}
			if p == c.directory+"/Containerfile" {
				selected, rebuild = true, true
				reasons = append(reasons, "recipe: "+p)
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
				if impact {
					reasons = append(reasons, "build context: "+p)
				}
				continue
			}
			affected, err := dependencies.affected(p)
			if err != nil {
				return r, err
			}
			if affected && sourcePath(p) {
				selected, rebuild = true, true
				reasons = append(reasons, "build input: "+p)
				continue
			}
			if strings.HasPrefix(p, c.directory+"/") {
				selected = true
				runtimeTest = true
				reasons = append(reasons, "action interface/test: "+p)
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
		runtimeTest = runtimeTest || rebuild
		for _, arch := range []string{"amd64", "arm64"} {
			runner := "ubuntu-24.04"
			if arch == "arm64" {
				runner += "-arm"
			}
			r.Include = append(r.Include, job{Image: c.image, Directory: c.directory, Arch: arch, Runner: runner, Rebuild: rebuild, Reference: c.reference, Scan: scan, RuntimeTest: runtimeTest, Reasons: reasons})
		}
	}
	return r, nil
}

// buildMatrix includes only selected product builds. Advisory tooling is published
// independently and never expands another image's build chain.
func buildMatrix(tests matrix) matrix {
	builds := matrix{Include: []job{}}
	for _, j := range tests.Include {
		if j.Rebuild && j.ArtifactID == "" {
			builds.Include = append(builds.Include, j)
		}
	}
	return builds
}

// A generated release commit only promotes references already qualified by the
// preparation run. Its tag owns the full published-image verification chain.
// A matching subject alone is never sufficient to suppress image work.
func releaseMetadataCommit(before, head string, components []component) bool {
	if !regexp.MustCompile(`^[a-f0-9]{40}$`).MatchString(before) {
		return false
	}
	parents, err := workflow.Git("rev-list", "--parents", "-n", "1", head)
	if err != nil || strings.TrimSpace(parents) != head+" "+before {
		return false
	}
	subject, err := workflow.Git("log", "-1", "--format=%s", head)
	if err != nil {
		return false
	}
	match := regexp.MustCompile(`^chore\(release\): (?:prepare )?((?:0|[1-9][0-9]*)\.(?:0|[1-9][0-9]*)\.(?:0|[1-9][0-9]*))$`).FindStringSubmatch(strings.TrimSpace(subject))
	if match == nil {
		return false
	}
	tag, err := workflow.Git("rev-parse", "--verify", "refs/tags/"+match[1]+"^{commit}")
	if err != nil || strings.TrimSpace(tag) != head {
		return false
	}
	owners := map[string]component{}
	for _, c := range components {
		owners[c.directory+"/action.yml"] = c
	}
	paths, err := workflow.Paths(before, head)
	if err != nil {
		return false
	}
	normalize := func(data string) string {
		data = catalog.ImageLine.ReplaceAllStringFunc(data, func(line string) string {
			return strings.SplitN(line, ": docker://", 2)[0] + ": <catalog-image>"
		})
		return strings.ReplaceAll(data, "  image: Containerfile\n", "  image: <catalog-image>\n")
	}
	changedPins := false
	for _, path := range paths {
		if path == "CHANGELOG.md" {
			continue
		}
		c, ok := owners[path]
		if !ok {
			return false
		}
		old, err := workflow.Git("show", before+":"+path)
		if err != nil {
			return false
		}
		next, err := workflow.Git("show", head+":"+path)
		if err != nil || normalize(old) != normalize(next) {
			return false
		}
		name, ref, err := catalog.Reference([]byte(next), c.directory)
		if err != nil || name != c.image || !catalog.Pinned.MatchString(ref) ||
			!strings.HasPrefix(ref, "ghcr.io/tdesaules/github-actions/") {
			return false
		}
		changedPins = true
	}
	return changedPins
}

func run() error {
	files := flag.String("files", workflow.Default("INPUT_FILES", "[]"), "changed paths JSON")
	full := flag.Bool("full", os.Getenv("INPUT_FULL") == "true", "select all images")
	output := flag.String("output", os.Getenv("MATRIX_OUTPUT"), "optional source-bound plan artifact")
	image := flag.String("image", os.Getenv("MATRIX_IMAGE"), "restrict a child workflow to one discovered image")
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
	if definition := os.Getenv("APPLICATION_DEFINITION"); definition != "" {
		return applicationPlan(definition, *repository, *event, *ref, paths, *full)
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
	if *event == "schedule" || *event == "maintenance" {
		if *ref != "refs/heads/main" {
			return fmt.Errorf("maintenance must target main")
		}
		mode = "maintenance"
		// Maintenance validates only its entry policy. Unpublished actions are
		// reported by advisory scanners and must not prevent Renovate or links.
		return workflow.Output("mode", mode)
	}
	version := "0.0.0-sha-" + head[:12]
	if strings.HasPrefix(*ref, "refs/tags/") {
		mode = "release"
		version = strings.TrimPrefix(*ref, "refs/tags/")
		if !semver.MatchString(version) || strings.ContainsAny(version, "-+") {
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
	releaseMetadata := mode == "development" && *ref == "refs/heads/main" && releaseMetadataCommit(*before, head, components)
	if *image != "" {
		filtered := []component{}
		for _, c := range components {
			if c.image == *image {
				filtered = append(filtered, c)
			}
		}
		if len(filtered) != 1 {
			return fmt.Errorf("unknown image %q", *image)
		}
		components = filtered
	}
	if mode == "release" {
		for _, c := range components {
			if !catalog.Pinned.MatchString(c.reference) {
				return fmt.Errorf("%s: released catalogue requires digest pins", c.directory)
			}
		}
	}
	m := matrix{Include: []job{}}
	if (mode == "development" || mode == "prepare") && !releaseMetadata {
		history := rebuildHistory{}
		if !*full && mode == "prepare" {
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
	if releaseMetadata {
		fmt.Fprintln(os.Stderr, "Generated release metadata commit: image qualification belongs to its matching tag; no development image chains")
	}
	if mode == "release" {
		for _, c := range components {
			for _, arch := range []string{"amd64", "arm64"} {
				runner := "ubuntu-24.04"
				if arch == "arm64" {
					runner += "-arm"
				}
				m.Include = append(m.Include, job{Image: c.image, Directory: c.directory, Arch: arch, Runner: runner, Reference: c.reference, Scan: true, RuntimeTest: true, Reasons: []string{"complete tag qualification"}})
			}
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
	allComponents, err := discover()
	if err != nil {
		return err
	}
	for _, c := range allComponents {
		if c.directory == "workflow/start-test-registry" {
			registry = c.reference
		}
	}
	builds := buildMatrix(m)
	type imageJob struct {
		Image     string `json:"image"`
		Directory string `json:"directory"`
	}
	runtimeTest := false
	for _, j := range m.Include {
		runtimeTest = runtimeTest || j.RuntimeTest
	}
	imageMatrix := struct {
		Include []imageJob `json:"include"`
	}{Include: []imageJob{}}
	seen := map[string]bool{}
	for _, j := range m.Include {
		if !seen[j.Image] {
			imageMatrix.Include = append(imageMatrix.Include, imageJob{j.Image, j.Directory})
			seen[j.Image] = true
			fmt.Fprintf(os.Stderr, "%s: rebuild=%t runtime=%t scan=%t reasons=%v\n", j.Image, j.Rebuild, j.RuntimeTest, j.Scan, j.Reasons)
		}
	}
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
	if *output != "" {
		if err := catalog.Write(*output, map[string]any{
			"schema_version": 1, "source_sha": head, "mode": mode,
			"before": *before, "version": version, "images": imageMatrix,
			"build_matrix": builds, "test_matrix": m, "release_metadata_only": releaseMetadata,
		}); err != nil {
			return err
		}
	}
	for name, value := range map[string]any{"mode": mode, "version": version, "runtime-test": runtimeTest, "source-sha": head, "image-matrix": imageMatrix, "matrix": m, "build-matrix": builds, "has-build": len(builds.Include) > 0, "has-work": len(m.Include) > 0, "test-registry-image": registry} {
		if err := workflow.Output(name, value); err != nil {
			return err
		}
	}
	return nil
}
func main() { workflow.Main("get-image-matrix 1", run) }
