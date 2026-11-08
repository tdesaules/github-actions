package main

import (
	"os"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"
)

// Selection changes must reach both consumers of /matrix. Publication must run
// the exact qualified planner, never silently substitute an older selector.
func TestPublicationPlannerActivationPreservesVerification(t *testing.T) {
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
	steps := map[string]map[string]any{}
	order := map[string]int{}
	for i, step := range document.Jobs["release"].Steps {
		name, _ := step["name"].(string)
		steps[name] = step
		order[name] = i
	}
	verify := steps["Verify qualified publication planner archive"]
	transfer := steps["Transfer qualified publication planner to private registry"]
	prepare := steps["Prepare qualified publication planner action"]
	candidate := steps["Validate exact-source publication artifacts with qualified planner"]
	published := steps["Validate exact-source publication artifacts with unchanged planner"]
	if verify["uses"] != "./workflow/test-build-artifact" || transfer["if"] != "steps.planner-receipt.outcome == 'success'" || prepare["if"] != "steps.planner-image.outcome == 'success'" || candidate["if"] != "steps.planner-image.outcome == 'success'" {
		t.Fatal("planner execution must depend on receipt verification and digest-preserving transfer")
	}
	if candidate["uses"] != "./.candidate-actions/workflow/new-publication-plan" || published["uses"] != "./workflow/new-publication-plan" {
		t.Fatal("planner activation paths are incorrect")
	}
	with := verify["with"].(map[string]any)
	if with["revision"] != "${{ github.sha }}" || with["image"] != "workflow-new-publication-plan" || with["architecture"] != "amd64" || with["version"] != "${{ needs.check.outputs.version }}-${{ inputs.artifact-run-id || github.run_id }}" {
		t.Fatal("planner receipt must match exact source, platform, owner and originating run")
	}
	if !strings.Contains(with["archive"].(string), "inputs.artifact-run-id || github.run_id") || !strings.Contains(with["receipt"].(string), "inputs.artifact-run-id || github.run_id") {
		t.Fatal("planner archive and receipt must come from the same source run")
	}
	guard := verify["if"].(string)
	if !strings.Contains(guard, "hashFiles(") || published["if"] != strings.TrimSuffix(guard, "!= ''")+"== ''" {
		t.Fatal("published fallback must be mutually exclusive, not an error recovery path")
	}
	for _, step := range []map[string]any{verify, transfer, prepare, candidate, published} {
		if step["continue-on-error"] != nil {
			t.Fatal("functional planner validation must be blocking")
		}
	}
	names := []string{"Download tested matrix artifacts", "Verify qualified publication planner archive", "Transfer qualified publication planner to private registry", "Prepare qualified publication planner action", "Validate exact-source publication artifacts with qualified planner", "Publish exact archives and verify remote digests"}
	for i := 1; i < len(names); i++ {
		if order[names[i-1]] >= order[names[i]] {
			t.Fatalf("incorrect activation order: %s before %s", names[i-1], names[i])
		}
	}
	for _, step := range document.Jobs["check"].Steps {
		if step["id"] == "resume" && step["uses"] != "./.candidate-actions/workflow/test-artifact-source" {
			t.Fatal("artifact-source reuse still embeds the old published selector")
		}
	}
}
