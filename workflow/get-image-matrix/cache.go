package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strconv"

	"github.com/tdesaules/github-actions/shared/workflow"
)

type cacheRun struct {
	ID             int64  `json:"id"`
	SHA            string `json:"head_sha"`
	Branch         string `json:"head_branch"`
	Path           string `json:"path"`
	Event          string `json:"event"`
	Status         string `json:"status"`
	Attempt        int    `json:"run_attempt"`
	HeadRepository struct {
		FullName string `json:"full_name"`
	} `json:"head_repository"`
}

type cacheArtifact struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	Expired     bool   `json:"expired"`
	WorkflowRun struct {
		ID  int64  `json:"id"`
		SHA string `json:"head_sha"`
	} `json:"workflow_run"`
}

type cacheJob struct {
	Name       string `json:"name"`
	Conclusion string `json:"conclusion"`
	SHA        string `json:"head_sha"`
}

type buildCache struct {
	client                      *http.Client
	endpoint, token, repository string
}

func (c buildCache) get(path string, target any) error {
	req, err := http.NewRequest("GET", c.endpoint+"/repos/"+c.repository+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/vnd.github+json")
	response, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("GitHub cache request failed")
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("GitHub cache returned HTTP %d", response.StatusCode)
	}
	return json.NewDecoder(response.Body).Decode(target)
}

// A matching recipe is insufficient: include transitive COPY/ADD/bind sources,
// modes, removals and effective ignore changes. Never use an unavailable Git tree.
func equivalentSources(directory, base, head string) (bool, error) {
	paths, err := workflow.Paths(base, head)
	if err != nil {
		return false, err
	}
	deps, err := buildDependencies(directory)
	if err != nil {
		return false, err
	}
	if deps.uncertain {
		return false, nil
	}
	var ignores *ignoreChange
	if hasIgnorePath(paths) {
		ignores, err = loadIgnoreChange(base, paths)
		if err != nil {
			return false, err
		}
	}
	for _, path := range paths {
		if isIgnorePath(path) {
			changed, err := ignores.affects(deps)
			if err != nil || changed {
				return false, err
			}
			continue
		}
		if path == directory+"/Containerfile" || path == "oci/build-image/action.yml" {
			return false, nil
		}
		// Unlike coarse selection, cache identity includes documentation and action
		// metadata when the recipe actually copies them into a stage.
		changed, err := deps.affected(path)
		if err != nil || changed {
			return false, err
		}
	}
	return true, nil
}

func (c buildCache) artifacts(run cacheRun) (map[string]cacheArtifact, error) {
	result := map[string]cacheArtifact{}
	duplicates := map[string]bool{}
	seen := map[string]bool{}
	for page := 1; ; page++ {
		var response struct {
			Artifacts []cacheArtifact `json:"artifacts"`
		}
		if err := c.get(fmt.Sprintf("/actions/runs/%d/artifacts?per_page=100&page=%d", run.ID, page), &response); err != nil {
			return nil, err
		}
		for _, a := range response.Artifacts {
			if seen[a.Name] {
				duplicates[a.Name] = true
			}
			seen[a.Name] = true
			if a.ID > 0 && !a.Expired && a.WorkflowRun.ID == run.ID && a.WorkflowRun.SHA == run.SHA {
				result[a.Name] = a
			}
		}
		if len(response.Artifacts) < 100 {
			break
		}
	}
	for name := range duplicates {
		delete(result, name)
	}
	return result, nil
}

func (c buildCache) successfulJobs(run cacheRun) (map[string]bool, error) {
	result := map[string]bool{}
	for page := 1; ; page++ {
		var response struct {
			Jobs []cacheJob `json:"jobs"`
		}
		if err := c.get(fmt.Sprintf("/actions/runs/%d/attempts/%d/jobs?per_page=100&page=%d", run.ID, run.Attempt, page), &response); err != nil {
			return nil, err
		}
		for _, j := range response.Jobs {
			if j.SHA == run.SHA && j.Conclusion == "success" {
				result[j.Name] = true
			}
		}
		if len(response.Jobs) < 100 {
			break
		}
	}
	return result, nil
}

func (c buildCache) reuse(plan *matrix, head, branch string) error {
	if c.token == "" || !regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$`).MatchString(c.repository) {
		return fmt.Errorf("repository and token required for build reuse")
	}
	pending := false
	for _, j := range plan.Include {
		pending = pending || (j.Rebuild && j.Image != "workflow-new-vulnerability-review")
	}
	if !pending {
		return nil
	}
	var response struct {
		Runs []cacheRun `json:"workflow_runs"`
	}
	// Bounded cache window; a miss rebuilds rather than trusting an older artifact.
	if err := c.get("/actions/workflows/main.yml/runs?event=push&status=completed&per_page=20&branch="+url.QueryEscape(branch), &response); err != nil {
		return err
	}
	entries := append([]job(nil), plan.Include...)
	for _, run := range response.Runs {
		if run.ID < 1 || run.Attempt < 1 || !regexp.MustCompile(`^[a-f0-9]{40}$`).MatchString(run.SHA) || run.Status != "completed" || run.Event != "push" || run.Path != ".github/workflows/main.yml" || run.Branch != branch || run.HeadRepository.FullName != c.repository {
			continue
		}
		if _, err := workflow.Git("merge-base", "--is-ancestor", run.SHA, head); err != nil {
			continue
		}
		eligible := []int{}
		for i, j := range entries {
			if !j.Rebuild || j.ArtifactID != "" || j.Image == "workflow-new-vulnerability-review" {
				continue
			}
			ok, err := equivalentSources(j.Directory, run.SHA, head)
			if err != nil {
				return err
			}
			if ok {
				eligible = append(eligible, i)
			}
		}
		if len(eligible) == 0 {
			continue
		}
		artifacts, err := c.artifacts(run)
		if err != nil {
			return err
		}
		jobs, err := c.successfulJobs(run)
		if err != nil {
			return err
		}
		used := false
		for _, i := range eligible {
			j := &entries[i]
			name := fmt.Sprintf("build-%s-%s-%d", j.Image, j.Arch, run.ID)
			a, ok := artifacts[name]
			if !ok || !jobs[fmt.Sprintf("build (%s, %s)", j.Image, j.Arch)] {
				continue
			}
			j.ArtifactID, j.SourceRun = strconv.FormatInt(a.ID, 10), strconv.FormatInt(run.ID, 10)
			j.SourceRevision, j.SourceVersion = run.SHA, "0.0.0-sha-"+run.SHA[:12]+"-"+j.SourceRun
			used = true
		}
		if used {
			// Reject a concurrent rerun before exposing artifact IDs to the workflow.
			var current cacheRun
			if err := c.get(fmt.Sprintf("/actions/runs/%d", run.ID), &current); err != nil {
				return err
			}
			if current != run {
				return fmt.Errorf("build cache run changed during selection")
			}
		}
	}
	plan.Include = entries
	return nil
}
