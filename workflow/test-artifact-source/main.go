package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
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
	Status     string `json:"status"`
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
	plan := flag.String("release-plan", os.Getenv("INPUT_RELEASE-PLAN"), "release plan defining required images")
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
	client := &http.Client{Timeout: 60 * time.Second, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}
	endpoint := "https://api.github.com/repos/" + *repo + "/actions/runs/" + *id
	get := func(url string, target any) error {
		req, err := http.NewRequest("GET", url, nil)
		if err != nil {
			return err
		}
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Accept", "application/vnd.github+json")
		resp, err := client.Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			return fmt.Errorf("GitHub returned HTTP %d", resp.StatusCode)
		}
		return json.NewDecoder(resp.Body).Decode(target)
	}
	var source sourceRun
	if err := get(endpoint, &source); err != nil {
		return err
	}
	if err := validate(source, *revision); err != nil {
		return err
	}
	if source.Status != "completed" {
		return fmt.Errorf("artifact source is still running")
	}
	cmd := exec.Command("/matrix", "--event=workflow_dispatch", "--ref=refs/heads/main", "--release-plan="+*plan)
	cmd.Env = append(os.Environ(), "GITHUB_OUTPUT=")
	cmd.Stderr = os.Stderr
	data, err := cmd.Output()
	if err != nil {
		return err
	}
	var matrix struct {
		Include []struct {
			Image, Arch string
			Rebuild     bool
		}
	}
	found := false
	for _, line := range strings.Split(string(data), "\n") {
		if value, ok := strings.CutPrefix(line, "matrix="); ok {
			if err = json.Unmarshal([]byte(value), &matrix); err != nil {
				return err
			}
			found = true
		}
	}
	if !found {
		return fmt.Errorf("selector did not return a matrix")
	}
	expected := []string{}
	for _, job := range matrix.Include {
		if job.Rebuild {
			expected = append(expected, job.Image+"-"+job.Arch)
		}
	}
	artifacts := []artifact{}
	for page := 1; ; page++ {
		var response struct {
			Artifacts []artifact `json:"artifacts"`
		}
		if err = get(endpoint+"/artifacts?per_page=100&page="+strconv.Itoa(page), &response); err != nil {
			return err
		}
		artifacts = append(artifacts, response.Artifacts...)
		if len(response.Artifacts) < 100 {
			break
		}
	}
	ids, err := selectArtifacts(artifacts, expected, *id, source.Attempt)
	if err != nil {
		return err
	}
	var current sourceRun
	if err = get(endpoint, &current); err != nil {
		return err
	}
	if current != source {
		return fmt.Errorf("artifact source changed during selection")
	}
	return workflow.Output("artifact-ids", strings.Join(ids, ","))
}
func main() { workflow.Main("test-artifact-source 1", run) }
