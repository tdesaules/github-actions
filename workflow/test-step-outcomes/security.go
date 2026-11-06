package main

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"html"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type stepOutcome struct {
	Outcome string `json:"outcome"`
}
type securityResult struct {
	Operation    string         `json:"operation"`
	Outcome      string         `json:"outcome"`
	Status       string         `json:"status"`
	Subject      string         `json:"subject"`
	Component    string         `json:"component,omitempty"`
	Architecture string         `json:"architecture,omitempty"`
	Report       string         `json:"report,omitempty"`
	Counts       map[string]int `json:"severity_counts,omitempty"`
	Reason       string         `json:"reason,omitempty"`
}

// Report security separately; never reinterpret a continued failure's conclusion
// as success. This function cannot authorize functional qualification.
func securityResults(steps map[string]stepOutcome, subject, component, arch, report string) []securityResult {
	names := []string{}
	for name := range steps {
		if strings.HasPrefix(name, "security-") {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	results := []securityResult{}
	for _, name := range names {
		r := securityResult{Operation: name, Outcome: steps[name].Outcome, Status: "not_run", Subject: subject, Component: component, Architecture: arch}
		switch r.Outcome {
		case "failure":
			r.Status, r.Reason = "error", "Operation failed; see step diagnostics (not a clean result)"
		case "success":
			r.Status = "completed"
			if strings.Contains(name, "signatures-sign") {
				r.Reason = "Signing step completed; consult per-digest signing evidence (reused images may be skipped)"
			}
			if strings.Contains(name, "signatures-verify") {
				r.Status = "verified"
			}
		case "skipped", "":
			r.Reason = "Operation was not executed"
		default:
			r.Reason = "Operation did not complete: " + r.Outcome
		}
		if name == "security-scan" && report != "" {
			r.Report = report
			if r.Outcome == "success" {
				var document struct {
					Matches *[]struct {
						Vulnerability struct {
							Severity string `json:"severity"`
						} `json:"vulnerability"`
					} `json:"matches"`
				}
				data, err := os.ReadFile(report)
				if err == nil {
					err = json.Unmarshal(data, &document)
				}
				if err != nil || document.Matches == nil {
					r.Status, r.Reason = "error", "Successful scanner step has no valid Grype matches report"
				} else {
					r.Status, r.Counts = "clean", map[string]int{}
					for _, m := range *document.Matches {
						r.Counts[m.Vulnerability.Severity]++
						r.Status = "findings"
					}
				}
			}
		}
		results = append(results, r)
	}
	return results
}

func reportSecurity(steps map[string]stepOutcome) error {
	subject := os.Getenv("SECURITY_SUBJECT")
	if subject == "" && os.Getenv("SECURITY_REPORT") != "" {
		var sbom struct {
			Source struct {
				Metadata struct {
					ManifestDigest string `json:"manifestDigest"`
				} `json:"metadata"`
			} `json:"source"`
		}
		if data, err := os.ReadFile(filepath.Join(filepath.Dir(os.Getenv("SECURITY_REPORT")), "sbom.syft.json")); err == nil && json.Unmarshal(data, &sbom) == nil {
			subject = sbom.Source.Metadata.ManifestDigest
		}
	}
	if subject == "" && os.Getenv("SECURITY_RECEIPT") != "" {
		data, err := os.ReadFile(os.Getenv("SECURITY_RECEIPT"))
		if err != nil {
			return err
		}
		subject = fmt.Sprintf("receipt-sha256:%x", sha256.Sum256(data))
	}
	results := securityResults(steps, subject, os.Getenv("SECURITY_COMPONENT"), os.Getenv("SECURITY_ARCHITECTURE"), os.Getenv("SECURITY_REPORT"))
	if raw := os.Getenv("SECURITY_SUBJECTS"); raw != "" {
		var subjects map[string]string
		if err := json.Unmarshal([]byte(raw), &subjects); err != nil {
			return err
		}
		for i := range results {
			if value, ok := subjects[results[i].Operation]; ok {
				results[i].Subject = value
			}
		}
	}
	if len(results) == 0 {
		return nil
	}
	data, err := json.MarshalIndent(results, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(data))
	if path := os.Getenv("SECURITY_OUTPUT"); path != "" {
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			return err
		}
		if err := os.WriteFile(path, append(data, '\n'), 0644); err != nil {
			return err
		}
	}
	if path := os.Getenv("GITHUB_STEP_SUMMARY"); path != "" {
		file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0600)
		if err != nil {
			return err
		}
		defer file.Close()
		_, err = fmt.Fprintf(file, "\n### Advisory security results\n\nFunctional qualification is separate. `error`/`not_run` never means clean.\n\n<pre>%s</pre>\n", html.EscapeString(string(data)))
		return err
	}
	return nil
}
