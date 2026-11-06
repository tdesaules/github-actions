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
			Steps []struct{ Name, If string } `yaml:"steps"`
		} `yaml:"jobs"`
	}
	if err := yaml.Unmarshal(data, &document); err != nil {
		t.Fatal(err)
	}
	conditions := map[string]string{}
	for _, step := range document.Jobs["build"].Steps {
		conditions[step.Name] = step.If
	}
	type entry struct {
		image         string
		rebuild, scan bool
	}
	const reviewer = "workflow-new-vulnerability-review"
	for _, tc := range []struct {
		name    string
		entries []entry
		want    int
	}{
		{"empty", nil, 0},
		{"metadata", []entry{{"oci-export-image", false, false}}, 0},
		{"changed image", []entry{{"oci-export-image", true, true}}, 1},
		{"scan only", []entry{{"oci-export-image", false, true}}, 1},
		{"reviewer changed", []entry{{reviewer, true, true}}, 1},
		{"reviewer selected for scan", []entry{{"oci-export-image", false, true}, {reviewer, false, true}}, 1},
		{"reviewer metadata plus changed image", []entry{{"oci-export-image", true, true}, {reviewer, false, false}}, 1},
		{"first image reused", []entry{{"oci-export-image", false, false}, {"oci-new-sbom", true, true}}, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			build, selected := false, false
			for _, e := range tc.entries {
				build = build || e.rebuild || e.scan
				selected = selected || e.image == reviewer
			}
			producers := 0
			if build {
				for _, e := range tc.entries {
					for _, name := range []string{"Build selected native OCI archive", "Build source reviewer dependency"} {
						if name == "Build selected native OCI archive" && e.image != reviewer {
							continue
						}
						expression := strings.ReplaceAll(conditions[name], "'", "\"")
						expression = strings.NewReplacer(
							`contains(fromJSON(needs.check.outputs.matrix).include.*.image, "workflow-new-vulnerability-review")`, strconv.FormatBool(selected),
							"fromJSON(needs.check.outputs.matrix).include[0].image", strconv.Quote(tc.entries[0].image),
							"matrix.image", strconv.Quote(e.image), "matrix.rebuild", strconv.FormatBool(e.rebuild),
						).Replace(expression)
						result, err := types.Eval(token.NewFileSet(), nil, token.NoPos, strings.Join(strings.Fields(expression), " "))
						if err != nil {
							t.Fatal(err)
						}
						if constant.BoolVal(result.Value) {
							producers++
						}
					}
				}
			}
			if producers != tc.want {
				t.Fatalf("%d producers per architecture; want %d", producers, tc.want)
			}
		})
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
		prepared := map[string]bool{}
		if len(steps) == 0 {
			t.Fatalf("missing steps in %s", job)
		}
		for _, step := range steps {
			if _, exists := step["run"]; exists {
				t.Errorf("%s: host run step %v", job, step["name"])
			}
			uses, _ := step["uses"].(string)
			candidate := job == "test" && strings.HasPrefix(uses, "./.candidate-actions/")
			if uses == "./workflow/prepare-source-action" {
				with, _ := step["with"].(map[string]any)
				directory, _ := with["directory"].(string)
				if directory == "" {
					t.Fatal("source bootstrap lacks explicit directory")
				}
				prepared["./.source-actions/"+directory] = true
				continue // Composition of qualified archive operations, tested below.
			}
			if strings.HasPrefix(uses, "./.source-actions/") {
				if !prepared[uses] {
					t.Fatalf("%s: source action invoked before bootstrap: %s", job, uses)
				}
				candidate = true
				uses = strings.Replace(uses, "./.source-actions/", "./", 1)
			}
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
			bootstrap := candidate && action.Runs.Image == "docker://ghcr.io/tdesaules/github-actions/"+strings.ReplaceAll(strings.TrimPrefix(uses, "./"), "/", "-")+":unpublished"
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
		if job.Strategy.FailFast == nil || *job.Strategy.FailFast || job.Strategy.MaxParallel != 4 || job.Strategy.Matrix != "${{ fromJSON(needs.check.outputs.matrix) }}" {
			t.Fatalf("%s: qualification matrix constraints lost", name)
		}
	}
	if document.Jobs["build"].Env["VERSION"] != document.Jobs["test"].Env["VERSION"] || strings.Contains(document.Jobs["build"].Env["VERSION"], "run_attempt") {
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
			fixtures = i
		case "./workflow/test-candidate-runtime":
			runtime = i
		case "./workflow/test-reviewed-vulnerabilities", "./.source-actions/workflow/test-reviewed-vulnerabilities":
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
