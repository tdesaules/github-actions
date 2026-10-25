package workflow

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"
)

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
	for _, job := range []string{"check", "lint", "release", "maintenance"} {
		steps := workflow.Jobs[job].Steps
		if len(steps) == 0 {
			t.Fatalf("missing steps in %s", job)
		}
		for _, step := range steps {
			if _, exists := step["run"]; exists {
				t.Errorf("%s: host run step %v", job, step["name"])
			}
			uses, _ := step["uses"].(string)
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
			if action.Runs.Using != "docker" || !regexp.MustCompile(`^docker://.+@sha256:[a-f0-9]{64}$`).MatchString(action.Runs.Image) {
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
}
