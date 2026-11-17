package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"syscall"
	"testing"
)

func TestSBOMStreamingValidation(t *testing.T) {
	t.Chdir(t.TempDir())
	for _, tc := range []struct {
		name, document string
		valid          bool
	}{
		{"spdx", `{"spdxVersion":"SPDX-2.3","files":[{"name":"escaped\\\"value"}]}`, true},
		{"cyclone", `{"bomFormat":"CycloneDX","components":[]}`, true},
		{"syft", `{"artifacts":[]}`, true},
		{"null", `null`, false},
		{"array", `[]`, false},
		{"nested marker", `{"nested":{"spdxVersion":"SPDX-2.3"}}`, false},
		{"null artifacts", `{"artifacts":null}`, false},
		{"wrong marker type", `{"spdxVersion":[]}`, false},
		{"duplicate last wins", `{"artifacts":[],"artifacts":null}`, false},
		{"truncated payload", `{"spdxVersion":"SPDX-2.3","files":[`, false},
		{"invalid ignored payload", `{"spdxVersion":"SPDX-2.3","ignored":[tru]}`, false},
		{"bad escape", `{"spdxVersion":"SPDX-2.3","ignored":"\q"}`, false},
		{"trailing comma", `{"artifacts":[],}`, false},
		{"second document", `{"artifacts":[]} {}`, false},
		{"trailing garbage", `{"artifacts":[]} x`, false},
		{"huge string", `{"artifacts":[],"ignored":"` + strings.Repeat("x", maxSBOMToken+1) + `"}`, false},
		{"huge whitespace", `{"artifacts":[]}` + strings.Repeat(" ", maxSBOMToken+1), false},
		{"huge number", `{"artifacts":[],"ignored":` + strings.Repeat("1", maxSBOMToken+1) + `}`, false},
		{"deep nesting", `{"artifacts":` + strings.Repeat("[", maxSBOMDepth+1) + strings.Repeat("]", maxSBOMDepth+1) + `}`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := os.WriteFile("report.json", []byte(tc.document), 0600); err != nil {
				t.Fatal(err)
			}
			got := inspect("report.json", "success", false)
			if (got.Status == "clean") != tc.valid || (!tc.valid && got.Reason == "") {
				t.Fatalf("%+v", got)
			}
		})
	}
}

// Default catalog Go tests exercise all three formats beyond the old 32 MiB
// limit, including malformed tails after an otherwise recognizable report.
func TestLargeSBOMSummary(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("GITHUB_OUTPUT", "")
	t.Setenv("GITHUB_STEP_SUMMARY", "")
	for _, marker := range []string{`"spdxVersion":"SPDX-2.3"`, `"bomFormat":"CycloneDX"`, `"artifacts":[]`} {
		f, err := os.Create("large.json")
		if err != nil {
			t.Fatal(err)
		}
		w := bufio.NewWriter(f)
		fmt.Fprintf(w, `{%s,"payload":[`, marker)
		entry := `{"name":"` + strings.Repeat("p", 1000) + `","values":[true,null,1.25]}`
		for i := 0; i < 34000; i++ {
			if i > 0 {
				fmt.Fprint(w, ",")
			}
			fmt.Fprint(w, entry)
		}
		fmt.Fprint(w, `]}`)
		if err := w.Flush(); err != nil {
			t.Fatal(err)
		}
		if err := f.Close(); err != nil {
			t.Fatal(err)
		}
		info, _ := os.Stat("large.json")
		if info.Size() <= maxReport {
			t.Fatal("fixture does not reproduce old limit")
		}
		if err := os.WriteFile("grype.json", []byte(`{"matches":[{"vulnerability":{"severity":"High"}}]}`), 0600); err != nil {
			t.Fatal(err)
		}
		if err := execute([]string{"--subject=git:" + strings.Repeat("a", 40), "--sbom=large.json", "--report=grype.json", "--sbom-outcome=success", "--scan-outcome=success", "--output=summary.json"}); err != nil {
			t.Fatal(err)
		}
		raw, err := os.ReadFile("summary.json")
		if err != nil {
			t.Fatal(err)
		}
		var s summary
		if err := json.Unmarshal(raw, &s); err != nil {
			t.Fatal(err)
		}
		if s.SBOM.Status != "clean" || s.Scan.Status != "findings" || s.Scan.Counts["High"] != 1 || !s.Advisory {
			t.Fatalf("%+v", s)
		}
		if got := inspect("large.json", "failure", false); got.Status != "error" || got.Outcome != "failure" {
			t.Fatal(got)
		}
		if got := inspect("large.json", "success", true); got.Status != "error" {
			t.Fatal("Grype limit removed")
		}
		if err := os.Truncate("large.json", info.Size()-1); err != nil {
			t.Fatal(err)
		}
		if got := inspect("large.json", "success", false); got.Status != "error" {
			t.Fatal("truncated large report accepted")
		}
	}
}

func TestSBOMUnsafeFiles(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := os.WriteFile("valid.json", []byte(`{"artifacts":[]}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("valid.json", "symlink"); err != nil {
		t.Fatal(err)
	}
	if err := syscall.Mkfifo("fifo", 0600); err != nil {
		t.Fatal(err)
	}
	f, err := os.Create("oversize")
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Truncate(maxSBOM + 1); err != nil {
		t.Fatal(err)
	}
	f.Close()
	for _, path := range []string{"symlink", "fifo", ".", "missing", "oversize"} {
		if got := inspect(path, "success", false); got.Status != "error" || got.Reason == "" {
			t.Fatalf("%s: %+v", path, got)
		}
	}
}
