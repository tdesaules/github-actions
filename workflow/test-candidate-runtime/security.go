package main

import (
	"fmt"
	"strings"

	"github.com/tdesaules/github-actions/shared/candidate"
	"github.com/tdesaules/github-actions/shared/catalog"
)

func securityCases(root *candidate.Root, results *[]result) error {
	base := []string{"--subject=git:" + strings.Repeat("a", 40), "--sbom-outcome=success", "--scan-outcome=success", "--sbom=/fixtures/syft-empty.json", "--report=/fixtures/security-findings.json", "--output=security-summary.json"}
	if err := actionCase(root, results, "actual advisory High/Critical findings", "/reports", base, nil, true, ""); err != nil {
		return err
	}
	var s struct {
		Advisory bool
		Scan     struct {
			Status string
			Counts map[string]int `json:"severity_counts"`
		}
	}
	if err := catalog.Read("/reports/security-summary.json", &s); err != nil {
		return err
	}
	if !s.Advisory || s.Scan.Status != "findings" || s.Scan.Counts["Critical"] != 1 || s.Scan.Counts["High"] != 1 {
		return fmt.Errorf("advisory findings were lost")
	}
	if err := actionCase(root, results, "tool failure remains advisory error", "/reports", append(base, "--scan-outcome=failure", "--output=security-failure.json"), nil, true, ""); err != nil {
		return err
	}
	if err := catalog.Read("/reports/security-failure.json", &s); err != nil {
		return err
	}
	if s.Scan.Status != "error" {
		return fmt.Errorf("failed scan became clean")
	}
	return actionCase(root, results, "reject unsafe security output", "/reports", append(base, "--output=../unsafe-summary.json"), nil, false, "workspace-relative")
}
