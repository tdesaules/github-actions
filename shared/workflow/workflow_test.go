package workflow

import (
	"go/constant"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"
)

// Evaluate the workflow's actual status gates rather than a second routing
// implementation. These gates intentionally use only Boolean/string expressions.
func TestIncrementalJobGates(t *testing.T) {
	data, err := os.ReadFile("../../.github/workflows/main.yml")
	if err != nil {
		t.Fatal(err)
	}
	var document struct {
		Jobs map[string]struct {
			If    string `yaml:"if"`
			Steps []struct {
				ID   string            `yaml:"id"`
				With map[string]string `yaml:"with"`
			} `yaml:"steps"`
		} `yaml:"jobs"`
	}
	if err := yaml.Unmarshal(data, &document); err != nil {
		t.Fatal(err)
	}
	for _, step := range document.Jobs["check"].Steps {
		if step.ID == "select" && step.With["full"] != "${{ steps.sources.outputs.full }}" {
			t.Fatal("image selection must preserve the detector's diff/fallback decision")
		}
	}
	for _, tc := range []struct {
		name, mode                          string
		lint, build, test                   bool
		lintResult, buildResult, testResult string
		cancelled                           bool
		wantBuild, wantTest, wantRelease    bool
	}{
		{"empty", "development", false, false, false, "skipped", "skipped", "skipped", false, false, false, false},
		{"docs", "development", true, false, false, "success", "skipped", "skipped", false, false, false, false},
		{"metadata", "development", true, false, true, "success", "skipped", "success", false, false, true, false},
		{"source without lint", "development", false, true, true, "skipped", "success", "success", false, true, true, false},
		{"scan helper", "development", false, true, true, "skipped", "success", "success", false, true, true, false},
		{"prepare metadata", "prepare", true, false, true, "success", "skipped", "success", false, false, true, true},
		{"prepare candidate", "prepare", true, true, true, "success", "success", "success", false, true, true, true},
		{"skipped required lint", "prepare", true, true, true, "skipped", "success", "success", false, false, false, false},
		{"failed build", "prepare", true, true, true, "success", "failure", "skipped", false, true, false, false},
		{"skipped required build", "prepare", true, true, true, "success", "skipped", "success", false, true, false, false},
		{"skipped required test", "prepare", true, true, true, "success", "success", "skipped", false, true, true, false},
		{"failed test", "prepare", true, true, true, "success", "success", "failure", false, true, true, false},
		{"cancelled", "prepare", true, true, true, "success", "success", "success", true, false, false, false},
		{"tag", "release", true, false, false, "success", "skipped", "skipped", false, false, false, true},
		{"full qualification", "qualification", true, true, true, "success", "success", "success", false, true, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			values := map[string]string{
				"needs.check.result": "success", "needs.check.outputs.mode": tc.mode,
				"needs.check.outputs.has-lint":        strconv.FormatBool(tc.lint),
				"needs.check.outputs.has-build":       strconv.FormatBool(tc.build),
				"needs.check.outputs.has-work":        strconv.FormatBool(tc.test),
				"needs.check.outputs.release-changed": "true",
				"needs.lint.result":                   tc.lintResult, "needs.build.result": tc.buildResult, "needs.test.result": tc.testResult,
			}
			for job, want := range map[string]bool{"build": tc.wantBuild, "test": tc.wantTest, "release": tc.wantRelease} {
				expression := strings.ReplaceAll(document.Jobs[job].If, "'", "\"")
				expression = strings.Join(strings.Fields(expression), " ")
				expression = strings.ReplaceAll(expression, "always()", "true")
				expression = strings.ReplaceAll(expression, "cancelled()", strconv.FormatBool(tc.cancelled))
				for key, value := range values {
					expression = strings.ReplaceAll(expression, key, strconv.Quote(value))
				}
				result, err := types.Eval(token.NewFileSet(), nil, token.NoPos, expression)
				if err != nil {
					t.Fatalf("%s: %v", job, err)
				}
				if got := constant.BoolVal(result.Value); got != want {
					t.Errorf("%s: got %t, want %t", job, got, want)
				}
			}
		})
	}
}

func TestSourceReviewerHasExactlyOneArtifactProducer(t *testing.T) {
	data, err := os.ReadFile("../../.github/workflows/main.yml")
	if err != nil {
		t.Fatal(err)
	}
	var document struct {
		Jobs map[string]struct {
			Steps []map[string]any `yaml:"steps"`
		} `yaml:"jobs"`
	}
	if err := yaml.Unmarshal(data, &document); err != nil {
		t.Fatal(err)
	}
	producers := 0
	for _, step := range document.Jobs["build"].Steps {
		switch step["uses"] {
		case "./oci/build-image":
			producers++
			if step["if"] != nil {
				t.Fatal("filtered build entries must not silently skip construction")
			}
		case "./workflow/new-build-receipt":
			if step["if"] != "steps.archive.outcome == 'success'" {
				t.Fatal("receipt must require actual build success")
			}
		default:
			continue
		}
		if step["continue-on-error"] != "${{ matrix.dependency == true }}" {
			t.Fatal("only dependency-only reviewer builds may be advisory")
		}
	}
	if producers != 1 {
		t.Fatalf("%d producers per build row; want one", producers)
	}
	for _, step := range document.Jobs["test"].Steps {
		if step["name"] == "Download source reviewer archive" {
			with := step["with"].(map[string]any)
			if with["name"] != "build-workflow-new-vulnerability-review-${{ matrix.arch }}-${{ github.run_id }}" {
				t.Fatal("reviewer consumer must use the same build artifact for products and dependencies")
			}
		}
	}
}

// Keep migrated jobs action-composed: host execution must not creep back in.
func TestMigratedJobsUsePublishedActions(t *testing.T) {
	root := filepath.Join("..", "..")
	data, err := os.ReadFile(filepath.Join(root, ".github/workflows/main.yml"))
	if err != nil {
		t.Fatal(err)
	}
	var workflow struct {
		Jobs map[string]struct {
			Steps []map[string]any `yaml:"steps"`
		} `yaml:"jobs"`
	}
	if err := yaml.Unmarshal(data, &workflow); err != nil {
		t.Fatal(err)
	}
	for _, job := range []string{"check", "lint", "build", "test", "release", "maintenance"} {
		steps := workflow.Jobs[job].Steps
		if len(steps) == 0 {
			t.Fatalf("missing steps in %s", job)
		}
		for _, step := range steps {
			if _, exists := step["run"]; exists {
				t.Errorf("%s: host run step %v", job, step["name"])
			}
			uses, _ := step["uses"].(string)
			with, _ := step["with"].(map[string]any)
			env, _ := step["env"].(map[string]any)
			if uses == "./workflow/prepare-source-action" || strings.HasPrefix(uses, "./.source-actions/") || with["allow-source"] != nil || env["ALLOW_SOURCE_ACTIONS"] != nil {
				t.Fatalf("%s: retired source bootstrap in %v", job, step["name"])
			}
			candidate := (job == "test" || (job == "check" && uses == "./.candidate-actions/workflow/get-image-matrix")) && strings.HasPrefix(uses, "./.candidate-actions/")
			// Candidate actions preserve the source directory beneath this prefix.
			uses = strings.Replace(uses, "./.candidate-actions/", "./", 1)
			if !strings.HasPrefix(uses, "./") {
				if !strings.HasPrefix(uses, "actions/checkout@") && !strings.HasPrefix(uses, "actions/upload-artifact@") && !strings.HasPrefix(uses, "actions/download-artifact@") && !strings.HasPrefix(uses, "actions/create-github-app-token@") {
					t.Errorf("%s: unexpected non-container action %q", job, uses)
				}
				continue
			}
			data, err := os.ReadFile(filepath.Join(root, uses, "action.yml"))
			if err != nil {
				t.Fatal(err)
			}
			var action struct {
				Runs struct {
					Using string `yaml:"using"`
					Image string `yaml:"image"`
				} `yaml:"runs"`
			}
			if err := yaml.Unmarshal(data, &action); err != nil {
				t.Fatal(err)
			}
			if uses == "./oci/build-image" {
				continue // The qualified, narrowly scoped Buildah launcher.
			}
			// Generated test metadata points at the checksum-verified candidate,
			// not the source's unpublished bootstrap reference. Runtime calls in
			// every other job still require a real immutable published image.
			pinned := regexp.MustCompile(`^docker://.+@sha256:[a-f0-9]{64}$`).MatchString(action.Runs.Image)
			bootstrap := candidate && action.Runs.Image == "Containerfile"
			if action.Runs.Using != "docker" || !(pinned || bootstrap) {
				t.Errorf("%s: %s must use a published Docker image", job, uses)
			}
		}
	}
}

func TestBuildAndQualificationAreSeparate(t *testing.T) {
	data, err := os.ReadFile("../../.github/workflows/main.yml")
	if err != nil {
		t.Fatal(err)
	}
	var document struct {
		Jobs map[string]struct {
			Needs    any               `yaml:"needs"`
			Env      map[string]string `yaml:"env"`
			Strategy struct {
				FailFast    *bool `yaml:"fail-fast"`
				MaxParallel int   `yaml:"max-parallel"`
				Matrix      any   `yaml:"matrix"`
			} `yaml:"strategy"`
			Steps []map[string]any `yaml:"steps"`
		} `yaml:"jobs"`
	}
	if err = yaml.Unmarshal(data, &document); err != nil {
		t.Fatal(err)
	}
	if len(document.Jobs) != 6 {
		t.Fatalf("expected six explicit pipeline jobs, got %d", len(document.Jobs))
	}
	for _, name := range []string{"build", "test"} {
		job := document.Jobs[name]
		matrixOutput := "matrix"
		if name == "build" {
			matrixOutput = "build-matrix"
		}
		if job.Strategy.FailFast == nil || *job.Strategy.FailFast || job.Strategy.MaxParallel != 4 || job.Strategy.Matrix != "${{ fromJSON(needs.check.outputs."+matrixOutput+") }}" {
			t.Fatalf("%s: qualification matrix constraints lost", name)
		}
	}
	if document.Jobs["build"].Env["VERSION"] != "${{ needs.check.outputs.version }}-${{ github.run_id }}" || document.Jobs["test"].Env["VERSION"] != "${{ matrix.source-version || format('{0}-{1}', needs.check.outputs.version, github.run_id) }}" {
		t.Fatal("test-only retries must retain the built candidate version")
	}
	for job, dependency := range map[string]string{"test": "build", "release": "test"} {
		found := false
		for _, need := range document.Jobs[job].Needs.([]any) {
			found = found || need == dependency
		}
		if !found {
			t.Fatalf("%s does not depend on %s", job, dependency)
		}
	}
	for _, step := range document.Jobs["build"].Steps {
		uses, _ := step["uses"].(string)
		run, _ := step["run"].(string)
		if uses == "./workflow/new-image-metadata" || strings.Contains(run, " new-metadata") {
			t.Fatal("build must not emit tested qualification metadata")
		}
	}
	for _, step := range document.Jobs["test"].Steps {
		run, _ := step["run"].(string)
		if strings.Contains(run, "scripts/images.py build-image") {
			t.Fatal("test must not reconstruct candidate images")
		}
	}
	steps := document.Jobs["test"].Steps
	fixtures, runtime, scan, qualified := -1, -1, -1, -1
	checks := []int{}
	reviewerChecks, reviewerRuntime, reviewerTransfer, reviewerPrepare, reviewerUse := -1, -1, -1, -1, -1
	for i, step := range steps {
		with, _ := step["with"].(map[string]any)
		if archive, _ := with["archive"].(string); strings.HasPrefix(archive, ".build/reviewer/") {
			switch step["uses"] {
			case "./workflow/test-build-artifact":
				reviewerChecks = i
			case "./workflow/test-candidate-runtime":
				reviewerRuntime = i
			case "./workflow/import-candidate-image":
				reviewerTransfer = i
			}
			continue
		}
		if step["name"] == "Prepare source reviewer action" {
			reviewerPrepare = i
		}
		if step["name"] == "Derive evidence-backed vulnerability review" {
			reviewerUse = i
		}
		switch step["uses"] {
		case "./workflow/new-test-fixtures":
			if fixtures >= 0 || step["id"] != "fixtures" || step["if"] != nil {
				t.Fatal("one unconditional published fixture generator must supply candidate outputs")
			}
			fixtures = i
		case "./workflow/test-candidate-runtime":
			if runtime >= 0 || step["if"] != nil {
				t.Fatal("one unconditional published runtime qualifier must test each matrix entry")
			}
			runtime = i
		case "./workflow/test-reviewed-vulnerabilities":
			scan = i
		case "./workflow/test-build-artifact":
			checks = append(checks, i)
		}
		if step["name"] == "Record successful matrix result" {
			qualified = i
			if step["if"] != "matrix.rebuild" {
				t.Fatal("qualification marker must require successful preceding checks")
			}
		}
	}
	if fixtures < 0 || runtime <= fixtures {
		t.Fatal("runner-owned fixtures must precede root-owned reports, including metadata-only tests")
	}
	if scan <= runtime || len(checks) != 2 || checks[0] >= runtime || checks[1] <= scan || qualified <= checks[1] {
		t.Fatal("archive integrity must bracket tests and scans before qualification")
	}
	if reviewerChecks < 0 || reviewerRuntime <= reviewerChecks || reviewerTransfer <= reviewerRuntime || reviewerPrepare <= reviewerTransfer || reviewerUse <= reviewerPrepare || scan <= reviewerUse {
		t.Fatal("source reviewer must be verified and runtime-qualified before preparation and scanning")
	}
	for _, step := range steps[qualified+1:] {
		uses, _ := step["uses"].(string)
		if !strings.HasPrefix(uses, "actions/upload-artifact@") {
			t.Fatal("tests must not run after the qualification marker")
		}
	}
}
