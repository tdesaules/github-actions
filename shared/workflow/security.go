package workflow

import (
	"encoding/json"
	"fmt"
	"html"
	"os"
	"path/filepath"
)

// RecordScan preserves errors independently of report availability. The scanner
// caller still returns its real error; the workflow controls advisory execution.
func RecordScan(sbom, report, component, arch string, scanErr error) error {
	result := map[string]any{"tool": "grype", "operation": "scan", "component": component, "architecture": arch, "report": report, "status": "error"}
	var inventory struct {
		Source struct {
			Metadata struct {
				ManifestDigest string `json:"manifestDigest"`
			} `json:"metadata"`
		} `json:"source"`
	}
	if data, err := os.ReadFile(sbom); err == nil && json.Unmarshal(data, &inventory) == nil {
		result["subject_digest"] = inventory.Source.Metadata.ManifestDigest
	}
	var scan struct {
		Matches *[]struct {
			Vulnerability struct {
				Severity string `json:"severity"`
			} `json:"vulnerability"`
		} `json:"matches"`
	}
	data, err := os.ReadFile(report)
	if err == nil {
		err = json.Unmarshal(data, &scan)
	}
	if scanErr != nil {
		result["reason"] = scanErr.Error()
	} else if err != nil || scan.Matches == nil {
		result["reason"] = "Missing or invalid Grype report"
	} else {
		result["status"] = "clean"
	}
	if err == nil && scan.Matches != nil {
		counts := map[string]int{}
		for _, m := range *scan.Matches {
			counts[m.Vulnerability.Severity]++
		}
		result["severity_counts"] = counts
		if scanErr == nil && len(*scan.Matches) > 0 {
			result["status"] = "findings"
		}
	}
	data, err = json.MarshalIndent(result, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(data))
	if err = os.MkdirAll(filepath.Dir(report), 0755); err != nil {
		return err
	}
	if err = os.WriteFile(report+".status.json", append(data, '\n'), 0644); err != nil {
		return err
	}
	if path := os.Getenv("GITHUB_STEP_SUMMARY"); path != "" {
		f, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0600)
		if err != nil {
			return err
		}
		defer f.Close()
		_, err = fmt.Fprintf(f, "\n### Advisory Grype report\n<pre>%s</pre>\n", html.EscapeString(string(data)))
		return err
	}
	return nil
}
