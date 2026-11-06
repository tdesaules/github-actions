package workflow

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"
)

func TestScanReportingPreservesErrorsAndFindings(t *testing.T) {
	for _, tc := range []struct {
		name, data, status string
		failure            error
	}{
		{"clean", `{"matches":[]}`, "clean", nil},
		{"findings", `{"matches":[{"vulnerability":{"severity":"High"}}]}`, "findings", nil},
		{"outage", `{"matches":[]}`, "error", errors.New("database unavailable")},
		{"invalid", `{}`, "error", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			report := filepath.Join(dir, "grype.json")
			if err := os.WriteFile(report, []byte(tc.data), 0600); err != nil {
				t.Fatal(err)
			}
			t.Setenv("GITHUB_STEP_SUMMARY", "")
			if err := RecordScan("missing-sbom", report, "fixture", "amd64", tc.failure); err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile(report + ".status.json")
			if err != nil {
				t.Fatal(err)
			}
			var result map[string]any
			if err = json.Unmarshal(data, &result); err != nil {
				t.Fatal(err)
			}
			if result["status"] != tc.status {
				t.Fatal(result)
			}
		})
	}
}

func TestSourceBootstrapUsesOnlyQualifiedOperations(t *testing.T) {
	data, err := os.ReadFile("../../workflow/prepare-source-action/action.yml")
	if err != nil {
		t.Fatal(err)
	}
	// Validate the transport chain, including the checksum check before execution.
	text := string(data)
	last := -1
	for _, action := range []string{"./oci/build-image", "./workflow/new-build-receipt", "./workflow/test-build-artifact", "./workflow/test-candidate-runtime", "./workflow/import-candidate-image", "./workflow/new-candidate-action"} {
		i := strings.Index(text, "uses: "+action+"\n")
		if i <= last {
			t.Fatalf("missing/out-of-order bootstrap operation %s", action)
		}
		last = i
	}
	if strings.Contains(text, "run:") {
		t.Fatal("bootstrap must not add host commands")
	}
}

func TestSecurityContinuationDoesNotRelaxFunctionalGates(t *testing.T) {
	data, err := os.ReadFile("../../.github/workflows/main.yml")
	if err != nil {
		t.Fatal(err)
	}
	var document struct {
		Jobs map[string]struct {
			Continue bool             `yaml:"continue-on-error"`
			Steps    []map[string]any `yaml:"steps"`
		} `yaml:"jobs"`
	}
	if err = yaml.Unmarshal(data, &document); err != nil {
		t.Fatal(err)
	}
	checks := map[string]bool{}
	for name, job := range document.Jobs {
		if job.Continue {
			t.Fatalf("%s hides functional job failures", name)
		}
		for _, step := range job.Steps {
			id, _ := step["id"].(string)
			uses, _ := step["uses"].(string)
			if id == "security-scan" || id == "security-signatures-sign" || id == "security-signatures-verify" || id == "security-catalog-scan" {
				checks[id] = true
				if step["continue-on-error"] != true {
					t.Fatalf("%s must retain advisory execution", id)
				}
			}
			if uses == "./workflow/publish-catalog-images" || uses == "./workflow/new-image-metadata" || uses == "./.source-actions/workflow/test-catalog-publication" || uses == "./.source-actions/workflow/complete-release" {
				checks[uses] = true
				if step["continue-on-error"] == true {
					t.Fatalf("functional gate weakened: %s", uses)
				}
			}
		}
	}
	if len(checks) != 8 {
		t.Fatalf("missing security/functional contract checks: %v", checks)
	}
}
