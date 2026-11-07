package main

import (
	"crypto/rand"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"time"

	"github.com/tdesaules/github-actions/shared/workflow"
)

const maxReport = 32 << 20

type evidence struct {
	Status      string          `json:"status"`
	Outcome     string          `json:"outcome"`
	Path        string          `json:"path"`
	Reason      string          `json:"reason,omitempty"`
	Counts      map[string]int  `json:"severity_counts,omitempty"`
	Database    json.RawMessage `json:"database,omitempty"`
	DatabaseAge *int64          `json:"database_age_seconds,omitempty"`
}
type summary struct {
	SchemaVersion int      `json:"schema_version"`
	Subject       string   `json:"subject"`
	Advisory      bool     `json:"advisory"`
	SBOM          evidence `json:"sbom"`
	Scan          evidence `json:"scan"`
}

func readJSON(path string, value any) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Size() > maxReport {
		return fmt.Errorf("report must be a regular file of at most 32 MiB")
	}
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	info, err = f.Stat()
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Size() > maxReport {
		return fmt.Errorf("report must be a regular file of at most 32 MiB")
	}
	data, err := io.ReadAll(io.LimitReader(f, maxReport+1))
	if err != nil {
		return err
	}
	if len(data) > maxReport {
		return fmt.Errorf("report exceeds 32 MiB")
	}
	return json.Unmarshal(data, value)
}
func inspect(path, outcome string, scan bool) evidence {
	e := evidence{Status: "error", Outcome: outcome, Path: path}
	if outcome == "skipped" {
		e.Status = "not_run"
		e.Reason = "step skipped"
		return e
	}
	if outcome != "success" {
		e.Reason = "step outcome: " + outcome
		return e
	}
	var doc map[string]json.RawMessage
	if err := readJSON(path, &doc); err != nil {
		e.Reason = "missing or invalid report: " + err.Error()
		return e
	}
	if doc == nil {
		e.Reason = "report must be a JSON object"
		return e
	}
	if !scan {
		var spdx, bom string
		_ = json.Unmarshal(doc["spdxVersion"], &spdx)
		_ = json.Unmarshal(doc["bomFormat"], &bom)
		var artifacts []json.RawMessage
		syft := json.Unmarshal(doc["artifacts"], &artifacts) == nil && artifacts != nil
		if !strings.HasPrefix(spdx, "SPDX-") && bom != "CycloneDX" && !syft {
			e.Reason = "unrecognized SBOM document"
			return e
		}
		e.Status = "clean"
		return e
	}
	var matches []struct {
		Vulnerability struct {
			Severity string `json:"severity"`
		} `json:"vulnerability"`
	}
	if err := json.Unmarshal(doc["matches"], &matches); err != nil || matches == nil {
		e.Reason = "Grype matches must be an array"
		return e
	}
	e.Counts = map[string]int{"Critical": 0, "High": 0, "Medium": 0, "Low": 0, "Negligible": 0, "Unknown": 0}
	for _, match := range matches {
		severity := match.Vulnerability.Severity
		if _, ok := e.Counts[severity]; !ok {
			e.Reason = "invalid Grype vulnerability severity"
			e.Counts = nil
			return e
		}
		e.Counts[severity]++
	}
	var descriptor struct {
		DB json.RawMessage `json:"db"`
	}
	if json.Unmarshal(doc["descriptor"], &descriptor) == nil && len(descriptor.DB) > 0 && string(descriptor.DB) != "null" {
		e.Database = descriptor.DB
		var db struct {
			Built string `json:"built"`
		}
		if json.Unmarshal(descriptor.DB, &db) == nil {
			if built, err := time.Parse(time.RFC3339, db.Built); err == nil {
				age := int64(time.Since(built).Seconds())
				e.DatabaseAge = &age
			}
		}
	}
	e.Status = "clean"
	if len(matches) > 0 {
		e.Status = "findings"
	}
	return e
}
func validOutcome(value string) bool {
	return value == "success" || value == "failure" || value == "cancelled" || value == "skipped"
}
func outputPath(path string, inputs ...string) (string, error) {
	if !filepath.IsLocal(path) || filepath.Ext(path) != ".json" || strings.ContainsAny(path, "\x00\r\n") {
		return "", fmt.Errorf("output must be a workspace-relative JSON path")
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	for _, input := range inputs {
		p, err := filepath.Abs(input)
		if err != nil {
			return "", err
		}
		if p == absolute {
			return "", fmt.Errorf("output must not overwrite an input report")
		}
	}
	current := "."
	for _, part := range strings.Split(filepath.Clean(path), string(filepath.Separator)) {
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if err != nil && !os.IsNotExist(err) {
			return "", err
		}
		if err == nil && info.Mode()&os.ModeSymlink != 0 {
			return "", fmt.Errorf("output must not traverse symlinks")
		}
	}
	return absolute, nil
}
func render(s summary) string {
	text := fmt.Sprintf("## Advisory security report\n\nSubject: `%s`\n\n| Operation | Status | Step outcome |\n| --- | --- | --- |\n| SBOM | %s | %s |\n| Grype | %s | %s |\n\n", s.Subject, s.SBOM.Status, s.SBOM.Outcome, s.Scan.Status, s.Scan.Outcome)
	if s.Scan.Counts != nil {
		text += fmt.Sprintf("**Critical: %d · High: %d** · Medium: %d · Low: %d · Negligible: %d · Unknown: %d\n\n", s.Scan.Counts["Critical"], s.Scan.Counts["High"], s.Scan.Counts["Medium"], s.Scan.Counts["Low"], s.Scan.Counts["Negligible"], s.Scan.Counts["Unknown"])
	}
	for _, e := range []evidence{s.SBOM, s.Scan} {
		if e.Reason != "" {
			text += "- " + strings.NewReplacer("\n", " ", "\r", " ", "<", "&lt;", ">", "&gt;", "`", "'").Replace(e.Reason) + "\n"
		}
	}
	if s.Scan.DatabaseAge != nil {
		text += fmt.Sprintf("\nDatabase age: %d seconds (reported build timestamp).\n", *s.Scan.DatabaseAge)
	}
	return text + "\nSecurity findings and tool failures are advisory; this report does not confer functional qualification. SBOM clean means a valid SBOM was produced.\n"
}
func execute(args []string) error {
	flags := flag.NewFlagSet("summarize-security", flag.ContinueOnError)
	subject := flags.String("subject", os.Getenv("INPUT_SUBJECT"), "archive SHA-256 or git:SHA")
	sbomOutcome := flags.String("sbom-outcome", os.Getenv("INPUT_SBOM-OUTCOME"), "SBOM step outcome")
	scanOutcome := flags.String("scan-outcome", os.Getenv("INPUT_SCAN-OUTCOME"), "scan step outcome")
	sbom := flags.String("sbom", os.Getenv("INPUT_SBOM"), "SBOM path")
	report := flags.String("report", os.Getenv("INPUT_REPORT"), "Grype JSON path")
	output := flags.String("output", workflow.Default("INPUT_OUTPUT", ".build/security/summary.json"), "summary JSON path")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 || !regexp.MustCompile(`^(?:sha256:)?[a-f0-9]{64}$|^git:[a-f0-9]{40}$`).MatchString(*subject) || !validOutcome(*sbomOutcome) || !validOutcome(*scanOutcome) {
		return fmt.Errorf("valid subject and explicit GitHub step outcomes are required")
	}
	for _, path := range []string{*sbom, *report} {
		if path == "" || strings.ContainsAny(path, "\x00\r\n") {
			return fmt.Errorf("report paths are required")
		}
	}
	_, err := outputPath(*output, *sbom, *report)
	if err != nil {
		return err
	}
	if os.Getuid() == 0 {
		owner := "."
		if p := os.Getenv("GITHUB_OUTPUT"); p != "" {
			owner = p
		}
		info, err := os.Stat(owner)
		if err != nil {
			return err
		}
		stat := info.Sys().(*syscall.Stat_t)
		if stat.Uid != 0 {
			if err := syscall.Setgroups([]int{}); err != nil {
				return err
			}
			if err := syscall.Setgid(int(stat.Gid)); err != nil {
				return err
			}
			if err := syscall.Setuid(int(stat.Uid)); err != nil {
				return err
			}
		}
	}
	s := summary{SchemaVersion: 1, Subject: *subject, Advisory: true, SBOM: inspect(*sbom, *sbomOutcome, false), Scan: inspect(*report, *scanOutcome, true)}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	root, err := os.OpenRoot(".")
	if err != nil {
		return err
	}
	defer root.Close()
	relative, err := filepath.Rel(".", *output)
	if err != nil {
		return err
	}
	if err := root.MkdirAll(filepath.Dir(relative), 0755); err != nil {
		return fmt.Errorf("unusable output directory: %w", err)
	}
	temporary := filepath.Join(filepath.Dir(relative), ".security-"+rand.Text()+".json")
	f, err := root.OpenFile(temporary, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer root.Remove(temporary)
	_, err = f.Write(append(data, '\n'))
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	if err := root.Rename(temporary, relative); err != nil {
		return fmt.Errorf("unusable output: %w", err)
	}
	markdown := render(s)
	fmt.Print(markdown)
	if path := os.Getenv("GITHUB_STEP_SUMMARY"); path != "" {
		f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0600)
		if err == nil {
			_, err = f.WriteString(markdown)
			closeErr := f.Close()
			if err == nil {
				err = closeErr
			}
		}
		if err != nil {
			fmt.Fprintf(os.Stderr, "advisory: cannot write step summary: %v\n", err)
		}
	}
	return nil
}
func main() { workflow.Main("summarize-security 1", func() error { return execute(os.Args[1:]) }) }
