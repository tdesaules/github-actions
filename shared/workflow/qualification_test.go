package workflow

import (
	"go/constant"
	"go/token"
	"go/types"
	"os"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"
)

func TestFullQualificationNeverPlansOrPublishes(t *testing.T) {
	data, err := os.ReadFile("../../.github/workflows/main.yml")
	if err != nil {
		t.Fatal(err)
	}
	var document struct {
		Jobs map[string]struct {
			If      string            `yaml:"if"`
			Outputs map[string]string `yaml:"outputs"`
			Steps   []struct {
				Name, If string
				With     map[string]string `yaml:"with"`
			} `yaml:"steps"`
		} `yaml:"jobs"`
	}
	if err = yaml.Unmarshal(data, &document); err != nil {
		t.Fatal(err)
	}
	evaluate := func(expression string, maintenance, runner bool) bool {
		expression = strings.ReplaceAll(expression, "'", "\"")
		expression = strings.NewReplacer(
			"inputs.qualify-catalog", "true", "inputs.maintenance", map[bool]string{true: "true", false: "false"}[maintenance],
			"inputs.qualify-runner", map[bool]string{true: "true", false: "false"}[runner],
			"inputs.artifact-run-id", "\"123\"", "github.event_name", "\"workflow_dispatch\"",
			"always()", "true", "cancelled()", "false",
			"needs.check.result", "\"success\"", "needs.lint.result", "\"success\"",
			"needs.build.result", "\"success\"", "needs.test.result", "\"success\"",
			"needs.check.outputs.has-lint", "\"true\"", "needs.check.outputs.has-build", "\"true\"",
			"needs.check.outputs.has-work", "\"true\"", "needs.check.outputs.release-changed", "\"true\"",
			"needs.check.outputs.mode", "\"prepare\"",
		).Replace(expression)
		value, err := types.Eval(token.NewFileSet(), nil, token.NoPos, strings.Join(strings.Fields(expression), " "))
		if err != nil {
			t.Fatal(expression, err)
		}
		return constant.BoolVal(value.Value)
	}
	for _, maintenance := range []bool{false, true} {
		for _, runner := range []bool{false, true} {
			if !evaluate(document.Jobs["check"].If, maintenance, runner) {
				t.Fatal("full qualification must execute check")
			}
			if evaluate(document.Jobs["release"].If, maintenance, runner) || evaluate(document.Jobs["maintenance"].If, maintenance, runner) {
				t.Fatal("qualification selected a writing job")
			}
			for _, step := range document.Jobs["check"].Steps {
				switch step.Name {
				case "Plan semantic release", "Validate publication artifact source", "Create native runner fixtures", "Qualify the published Buildah launcher":
					if evaluate(step.If, maintenance, runner) {
						t.Fatal("qualification selected incompatible step", step.Name)
					}
				case "Select mode and dynamic matrix":
					if !evaluate(step.If, maintenance, runner) {
						t.Fatal("full matrix selection skipped")
					}
					if step.With["event"] != "${{ github.event_name }}" || step.With["ref"] != "${{ github.ref }}" {
						t.Fatal("selector must retain dispatch/main validation")
					}
				}
			}
		}
	}
	if document.Jobs["check"].Outputs["mode"] != "${{ inputs.qualify-catalog && 'qualification' || steps.select.outputs.mode }}" {
		t.Fatal("qualification mode is not propagated")
	}
}
