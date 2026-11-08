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

func TestManualDispatchOnlyPreparesRelease(t *testing.T) {
	data, err := os.ReadFile("../../.github/workflows/main.yml")
	if err != nil {
		t.Fatal(err)
	}
	var document struct {
		On   map[string]any `yaml:"on"`
		Jobs map[string]struct {
			If      string            `yaml:"if"`
			Outputs map[string]string `yaml:"outputs"`
			Steps   []struct {
				Name, If, Uses string
				With           map[string]string `yaml:"with"`
			} `yaml:"steps"`
		} `yaml:"jobs"`
	}
	if err = yaml.Unmarshal(data, &document); err != nil {
		t.Fatal(err)
	}
	dispatch, ok := document.On["workflow_dispatch"].(map[string]any)
	if !ok || len(dispatch) != 0 || strings.Contains(string(data), "inputs.") {
		t.Fatal("manual dispatch must have no options or remaining input references")
	}
	if len(document.Jobs) != 6 || document.On["schedule"] == nil {
		t.Fatal("keep all six jobs and scheduled maintenance")
	}
	evaluate := func(expression, event string) bool {
		expression = strings.ReplaceAll(expression, "'", "\"")
		expression = strings.ReplaceAll(expression, "github.event_name", "\""+event+"\"")
		value, err := types.Eval(token.NewFileSet(), nil, token.NoPos, strings.Join(strings.Fields(expression), " "))
		if err != nil {
			t.Fatal(expression, err)
		}
		return constant.BoolVal(value.Value)
	}
	for _, event := range []string{"push", "workflow_dispatch", "schedule"} {
		if evaluate(document.Jobs["check"].If, event) != (event != "schedule") || evaluate(document.Jobs["maintenance"].If, event) != (event == "schedule") {
			t.Fatalf("incorrect check/maintenance routing for %s", event)
		}
		planFound, selectorFound := false, false
		for _, step := range document.Jobs["check"].Steps {
			switch step.Name {
			case "Plan semantic release":
				planFound = true
				if evaluate(step.If, event) != (event == "workflow_dispatch") {
					t.Fatal("only manual dispatch should plan releases")
				}
			case "Select mode and dynamic matrices":
				selectorFound = true
				if step.With["event"] != "${{ github.event_name }}" || step.With["ref"] != "${{ github.ref }}" {
					t.Fatal("selector must retain dispatch/main validation")
				}
			}
		}
		if !planFound || !selectorFound {
			t.Fatal("release planning and mode validation are required")
		}
	}
	if document.Jobs["check"].Outputs["mode"] != "${{ steps.select.outputs.mode }}" {
		t.Fatal("selector mode must be propagated")
	}
	renovateFound := false
	for _, step := range document.Jobs["maintenance"].Steps {
		if step.Uses == "./maintenance/update-dependencies" {
			renovateFound = true
			if step.With["dry-run"] != "false" {
				t.Fatal("scheduled Renovate must perform updates")
			}
		}
	}
	if !renovateFound {
		t.Fatal("scheduled Renovate was removed")
	}
}
