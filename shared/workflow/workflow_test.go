package workflow

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"
)

// Keep source checks action-composed: host execution must not creep back in.
func TestCheckAndLintUsePublishedActions(t *testing.T) {
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
	for _, job := range []string{"check", "lint"} {
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
				if !strings.HasPrefix(uses, "actions/checkout@") && !strings.HasPrefix(uses, "actions/upload-artifact@") && !strings.HasPrefix(uses, "actions/download-artifact@") {
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
