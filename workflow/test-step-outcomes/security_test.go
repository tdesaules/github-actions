package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSecurityOutcomesNeverConvertFailuresToClean(t *testing.T) {
	path := filepath.Join(t.TempDir(), "grype.json")
	if err := os.WriteFile(path, []byte(`{"matches":[{"vulnerability":{"severity":"Critical"}}]}`), 0600); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ outcome, status string }{{"success", "findings"}, {"failure", "error"}, {"skipped", "not_run"}} {
		r := securityResults(map[string]stepOutcome{"security-scan": {Outcome: tc.outcome}}, "image@sha256:abc", "fixture", "amd64", path)
		if len(r) != 1 || r[0].Status != tc.status {
			t.Fatalf("%s: %+v", tc.outcome, r)
		}
		if tc.outcome == "success" && r[0].Counts["Critical"] != 1 {
			t.Fatal("missing critical count")
		}
	}
	for _, data := range []string{`{}`, `{"matches":null}`, `not JSON`} {
		if err := os.WriteFile(path, []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
		r := securityResults(map[string]stepOutcome{"security-scan": {Outcome: "success"}}, "", "", "", path)
		if r[0].Status != "error" {
			t.Fatal("missing/corrupt evidence claimed clean", r)
		}
	}
}
