package workflow

import (
	"os"
	"testing"

	"go.yaml.in/yaml/v3"
)

func TestEntryWorkflowRouting(t *testing.T) {
	type job struct {
		Uses  string            `yaml:"uses"`
		With  map[string]string `yaml:"with"`
		Steps []struct {
			Name string            `yaml:"name"`
			Uses string            `yaml:"uses"`
			With map[string]string `yaml:"with"`
		} `yaml:"steps"`
	}
	type document struct {
		On   map[string]any `yaml:"on"`
		Jobs map[string]job `yaml:"jobs"`
	}
	read := func(name string) document {
		t.Helper()
		data, err := os.ReadFile("../../.github/workflows/" + name + ".yml")
		if err != nil {
			t.Fatal(err)
		}
		var doc document
		if err := yaml.Unmarshal(data, &doc); err != nil {
			t.Fatal(err)
		}
		return doc
	}
	main := read("main")
	push := main.On["push"].(map[string]any)
	if len(main.On) != 1 || push["branches"] == nil || push["tags"] != nil || main.Jobs["pipeline"].With["mode"] != "development" {
		t.Fatal("main must route only branch pushes to development")
	}
	release := read("release")
	push = release.On["push"].(map[string]any)
	if len(release.On) != 2 || push["tags"] == nil || push["branches"] != nil || release.On["workflow_dispatch"] == nil {
		t.Fatal("release must own tags and manual preparation")
	}
	maintenance := read("maintenance")
	if len(maintenance.On) != 2 || maintenance.On["schedule"] == nil || maintenance.On["workflow_dispatch"] == nil || len(maintenance.Jobs) != 1 {
		t.Fatal("maintenance must be independent and manually callable")
	}
	foundPolicy, foundRenovate := false, false
	for _, step := range maintenance.Jobs["maintenance"].Steps {
		if step.Name == "Validate maintenance ref" {
			foundPolicy = step.Uses == "./workflow/get-image-matrix" && step.With["event"] == "maintenance"
		}
		if step.Uses == "./maintenance/update-dependencies" {
			foundRenovate = step.With["dry-run"] == "false"
		}
	}
	if !foundPolicy || !foundRenovate {
		t.Fatal("maintenance needs explicit ref validation and real Renovate execution")
	}
	parent := read("catalog")
	if parent.Jobs["images"].Uses != "./.github/workflows/image.yml" {
		t.Fatal("parent must delegate image chains to the reusable workflow")
	}
	for _, doc := range []document{main, release, maintenance, parent} {
		if doc.On["pull_request"] != nil {
			t.Fatal("PR execution was not requested")
		}
	}
}
