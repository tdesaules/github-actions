package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

// This checks the report written by the actual native candidate Docker action.
// Calling execute/inspect here would only retest source and miss a stale image
// that exits zero while reporting the >32 MiB SBOM as an advisory error.
func TestHostedLargeSBOMCandidateReport(t *testing.T) {
	if os.Getenv("ATOMIC_SUMMARY_HOSTED_FIXTURE") != "1" {
		t.Skip("native candidate report assertion requires explicit fixture environment")
	}
	required := func(name string) string {
		t.Helper()
		value := os.Getenv(name)
		if value == "" {
			t.Fatalf("%s is required for hosted fixture verification", name)
		}
		return value
	}
	resolve := func(path string) string {
		t.Helper()
		if filepath.IsAbs(path) {
			return filepath.Clean(path)
		}
		if !filepath.IsLocal(path) {
			t.Fatal("fixture path must remain workspace-relative")
		}
		workspace := required("GITHUB_WORKSPACE")
		if !filepath.IsAbs(workspace) {
			t.Fatal("GITHUB_WORKSPACE must be absolute")
		}
		return filepath.Join(workspace, path)
	}
	source := resolve(required("ATOMIC_SUMMARY_SBOM"))
	report := resolve(required("ATOMIC_SUMMARY_REPORT"))
	expected := required("ATOMIC_SUMMARY_SBOM_SHA256")
	if !regexp.MustCompile(`^[a-f0-9]{64}$`).MatchString(expected) {
		t.Fatal("original fixture checksum must be a full SHA-256")
	}
	info, err := os.Lstat(source)
	if err != nil {
		t.Fatal(err)
	}
	if !info.Mode().IsRegular() || info.Size() <= 32<<20 || info.Size() > 36<<20 {
		t.Fatalf("expected a regular ~34 MiB source fixture, got %v", info)
	}
	reportInfo, err := os.Lstat(report)
	if err != nil {
		t.Fatal(err)
	}
	if !reportInfo.Mode().IsRegular() || reportInfo.Size() > 1<<20 {
		t.Fatal("candidate summary must be a small regular report")
	}
	raw, err := os.ReadFile(report)
	if err != nil {
		t.Fatal(err)
	}
	var result summary
	if err := json.Unmarshal(raw, &result); err != nil {
		t.Fatal(err)
	}
	if result.SchemaVersion != 1 || !result.Advisory ||
		result.Subject != required("ATOMIC_SUMMARY_SUBJECT") {
		t.Fatal("candidate report has incorrect schema, advisory marker or subject")
	}
	if result.SBOM.Status != "clean" || result.SBOM.Outcome != "success" || result.SBOM.Reason != "" {
		t.Fatalf("actual candidate did not accept the large successful SBOM: %+v", result.SBOM)
	}
	if result.Scan.Status != "not_run" || result.Scan.Outcome != "skipped" {
		t.Fatalf("skipped scan was misrepresented: %+v", result.Scan)
	}
	if resolve(result.SBOM.Path) != source {
		t.Fatal("candidate report is bound to a different SBOM path")
	}
	f, err := os.Open(source)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, f); err != nil {
		t.Fatal(err)
	}
	if hex.EncodeToString(hash.Sum(nil)) != expected {
		t.Fatal("candidate action modified the original large SBOM")
	}
}
