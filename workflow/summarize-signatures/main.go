package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/tdesaules/github-actions/shared/workflow"
)

type status struct {
	Status  string `json:"status"`
	Outcome string `json:"outcome"`
	Reason  string `json:"reason,omitempty"`
}

func validOutcome(value string) bool {
	return value == "success" || value == "failure" || value == "cancelled" || value == "skipped"
}

func signing(outcome string, signed bool) status {
	switch {
	case outcome == "failure":
		return status{"error", outcome, "signing action failed"}
	case outcome != "success":
		return status{"not_run", outcome, "signing did not complete"}
	case !signed:
		return status{"not_run", outcome, "signing disabled or no signature emitted"}
	default:
		return status{"signed", outcome, ""}
	}
}

func verification(outcome, path, subject string) status {
	if outcome == "failure" {
		return status{"error", outcome, "verification action failed"}
	}
	if outcome != "success" {
		return status{"not_run", outcome, "verification did not run"}
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return status{"error", outcome, "verification report is unavailable"}
	}
	var claims []struct {
		Critical struct {
			Type  string `json:"type"`
			Image struct {
				Digest string `json:"docker-manifest-digest"`
			} `json:"image"`
			Identity struct {
				Reference string `json:"docker-reference"`
			} `json:"identity"`
		} `json:"critical"`
	}
	if err := json.Unmarshal(data, &claims); err != nil {
		return status{"error", outcome, "verification report is not valid Cosign JSON"}
	}
	repository, digest, ok := strings.Cut(subject, "@")
	if !ok {
		return status{"error", outcome, "subject lacks immutable digest"}
	}
	for _, claim := range claims {
		if claim.Critical.Type == "cosign container image signature" &&
			claim.Critical.Image.Digest == digest && claim.Critical.Identity.Reference == repository {
			return status{"verified", outcome, ""}
		}
	}
	return status{"error", outcome, "verification report does not bind the expected subject"}
}

func run() error {
	subject := flag.String("subject", "", "immutable OCI subject")
	signOutcome := flag.String("sign-outcome", "skipped", "actual signing step outcome")
	signed := flag.Bool("signed", false, "actual signing action output")
	verifyOutcome := flag.String("verify-outcome", "skipped", "actual verification step outcome")
	reportPath := flag.String("report", "", "Cosign verification JSON path")
	output := flag.String("output", ".build/security/signatures.json", "summary path")
	flag.Parse()
	if flag.NArg() != 0 || !validOutcome(*signOutcome) || !validOutcome(*verifyOutcome) ||
		!regexp.MustCompile(`^ghcr\.io/[a-z0-9][a-z0-9._/-]+@sha256:[a-f0-9]{64}$`).MatchString(*subject) {
		return fmt.Errorf("immutable GHCR subject and actual step outcomes required")
	}
	if *output == "" || filepath.IsAbs(*output) || filepath.Clean(*output) == "." ||
		filepath.Clean(*output) == ".." || strings.HasPrefix(filepath.Clean(*output), "../") {
		return fmt.Errorf("output must be workspace-relative")
	}
	sign := signing(*signOutcome, *signed)
	verify := verification(*verifyOutcome, *reportPath, *subject)
	evidence := map[string]any{
		"schema_version": 1, "subject": *subject, "tool": "cosign",
		"recorded_at": time.Now().UTC().Format(time.RFC3339),
		"signing":     sign, "verification": verify,
		"run_id": os.Getenv("GITHUB_RUN_ID"), "run_attempt": os.Getenv("GITHUB_RUN_ATTEMPT"),
	}
	data, err := json.MarshalIndent(evidence, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(*output), 0755); err != nil {
		return err
	}
	if err := os.WriteFile(*output, append(data, '\n'), 0644); err != nil {
		return err
	}
	if sign.Status == "error" || verify.Status != "verified" {
		fmt.Printf("::warning::Advisory signature status for %s: signing=%s verification=%s\n", *subject, sign.Status, verify.Status)
	}
	return workflow.Output("report", *output)
}

func main() { workflow.Main("summarize-signatures 1", run) }
