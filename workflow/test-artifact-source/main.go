package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"regexp"
	"time"

	"github.com/tdesaules/github-actions/shared/workflow"
)

type sourceRun struct {
	Revision   string `json:"head_sha"`
	Branch     string `json:"head_branch"`
	Path       string `json:"path"`
	Event      string `json:"event"`
	Conclusion string `json:"conclusion"`
	Attempt    int    `json:"run_attempt"`
}

func validate(run sourceRun, revision string) error {
	if run.Revision != revision || run.Branch != "main" || run.Path != ".github/workflows/main.yml" || (run.Event != "push" && run.Event != "workflow_dispatch") || run.Conclusion != "success" || run.Attempt < 1 {
		return fmt.Errorf("artifact source must be a successful catalogue run at this exact main commit")
	}
	return nil
}
func run() error {
	id := flag.String("run-id", os.Getenv("INPUT_RUN-ID"), "artifact source run ID")
	repo := flag.String("repository", workflow.Default("INPUT_REPOSITORY", os.Getenv("GITHUB_REPOSITORY")), "owner/repository")
	revision := flag.String("revision", workflow.Default("INPUT_REVISION", os.Getenv("GITHUB_SHA")), "exact source revision")
	flag.Parse()
	if flag.NArg() != 0 {
		return fmt.Errorf("unexpected arguments")
	}
	if !regexp.MustCompile(`^[0-9]+$`).MatchString(*id) || !regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$`).MatchString(*repo) || !regexp.MustCompile(`^[a-f0-9]{40}$`).MatchString(*revision) {
		return fmt.Errorf("invalid run, repository or revision")
	}
	token := os.Getenv("INPUT_TOKEN")
	if token == "" {
		return fmt.Errorf("token is required")
	}
	req, err := http.NewRequest("GET", "https://api.github.com/repos/"+*repo+"/actions/runs/"+*id, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github+json")
	client := &http.Client{Timeout: 60 * time.Second, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("GitHub returned HTTP %d", resp.StatusCode)
	}
	var source sourceRun
	if err := json.NewDecoder(resp.Body).Decode(&source); err != nil {
		return err
	}
	if err := validate(source, *revision); err != nil {
		return err
	}
	return workflow.Output("artifact-pattern", fmt.Sprintf("oci-*-%s-%d", *id, source.Attempt))
}
func main() { workflow.Main("test-artifact-source 1", run) }
