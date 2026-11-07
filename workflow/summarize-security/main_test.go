package main

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestAdvisoryOutcomes(t *testing.T) {
	for _, tc := range []struct{ name, outcome, report, status string }{
		{"findings", "success", `{"matches":[{"vulnerability":{"severity":"Critical"}},{"vulnerability":{"severity":"High"}}],"descriptor":{"db":{"built":"2026-01-01T00:00:00Z","schemaVersion":6}}}`, "findings"},
		{"clean", "success", `{"matches":[]}`, "clean"},
		{"missing", "success", "", "error"},
		{"invalid", "success", `{"matches":null}`, "error"},
		{"missing matches", "success", `{}`, "error"},
		{"invalid severity", "success", `{"matches":[{"vulnerability":{}}]}`, "error"},
		{"malformed", "success", `{`, "error"},
		{"failed stale clean report", "failure", `{"matches":[]}`, "error"},
		{"cancelled", "cancelled", `{"matches":[]}`, "error"},
		{"skipped", "skipped", "", "not_run"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Chdir(t.TempDir())
			t.Setenv("GITHUB_OUTPUT", "")
			t.Setenv("GITHUB_STEP_SUMMARY", "")
			if tc.status == "findings" {
				if err := os.WriteFile("step-summary.md", nil, 0644); err != nil {
					t.Fatal(err)
				}
				t.Setenv("GITHUB_STEP_SUMMARY", "step-summary.md")
			}
			if err := os.WriteFile("sbom.json", []byte(`{"artifacts":[]}`), 0644); err != nil {
				t.Fatal(err)
			}
			if tc.report != "" {
				if err := os.WriteFile("grype.json", []byte(tc.report), 0644); err != nil {
					t.Fatal(err)
				}
			}
			err := execute([]string{"--subject=git:" + strings.Repeat("a", 40), "--sbom-outcome=success", "--scan-outcome=" + tc.outcome, "--sbom=sbom.json", "--report=grype.json"})
			if err != nil {
				t.Fatalf("advisory result failed action: %v", err)
			}
			data, err := os.ReadFile(".build/security/summary.json")
			if err != nil {
				t.Fatal(err)
			}
			var s summary
			if err := json.Unmarshal(data, &s); err != nil {
				t.Fatal(err)
			}
			if s.Scan.Status != tc.status || s.SBOM.Status != "clean" || !s.Advisory {
				t.Fatalf("wrong actual outcomes: %+v", s)
			}
			if tc.status == "findings" {
				if s.Scan.Counts["Critical"] != 1 || s.Scan.Counts["High"] != 1 || s.Scan.DatabaseAge == nil || !strings.Contains(render(s), "**Critical: 1 · High: 1**") {
					t.Fatal("findings/freshness not reported")
				}
				markdown, err := os.ReadFile("step-summary.md")
				if err != nil || !strings.Contains(string(markdown), "**Critical: 1 · High: 1**") {
					t.Fatalf("missing Markdown evidence: %v", err)
				}
			}
		})
	}
}
func TestSBOMFailuresAndUnsafeOutput(t *testing.T) {
	t.Chdir(t.TempDir())
	for outcome, want := range map[string]string{"success": "error", "failure": "error", "skipped": "not_run"} {
		if got := inspect("missing.json", outcome, false); got.Status != want {
			t.Fatal(got)
		}
	}
	if err := os.WriteFile("invalid.json", []byte(`{}`), 0644); err != nil {
		t.Fatal(err)
	}
	if got := inspect("invalid.json", "success", false); got.Status != "error" {
		t.Fatal(got)
	}
	if err := os.Symlink(t.TempDir(), "linked"); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"../outside.json", "/tmp/outside.json", "linked/output.json", "input.json"} {
		if _, err := outputPath(path, "input.json"); err == nil {
			t.Fatalf("unsafe output accepted: %s", path)
		}
	}
	if err := execute([]string{"--subject=invalid", "--sbom-outcome=success", "--scan-outcome=success", "--sbom=a", "--report=b"}); err == nil {
		t.Fatal("invalid subject accepted")
	}
}
