package pantavisor

import (
	"encoding/json"
	"fmt"
	"github.com/tdesaules/github-actions/shared/workflow"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"time"
)

func githubRead(path string, value any) error {
	repository, token := os.Getenv("GITHUB_REPOSITORY"), os.Getenv("INPUT_TOKEN")
	if !regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$`).MatchString(repository) || token == "" {
		return fmt.Errorf("repository and GitHub token required")
	}
	req, err := http.NewRequest("GET", "https://api.github.com/repos/"+repository+"/"+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github+json")
	client := http.Client{Timeout: 30 * time.Second}
	response, err := client.Do(req)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		return fmt.Errorf("GitHub read failed: HTTP %d", response.StatusCode)
	}
	return json.NewDecoder(response.Body).Decode(value)
}

func QualificationArtifacts() error {
	build := Input("build-artifact-id", "")
	if err := Parse(); err != nil {
		return err
	}
	run := os.Getenv("GITHUB_RUN_ID")
	if !regexp.MustCompile(`^[1-9][0-9]*$`).MatchString(*build) || !regexp.MustCompile(`^[1-9][0-9]*$`).MatchString(run) {
		return fmt.Errorf("build artifact ID and current run are required")
	}
	pattern := regexp.MustCompile(`^tested-system-(base|k0s)-` + regexp.QuoteMeta(*build) + `-[1-9][0-9]*$`)
	latest := map[string]int64{}
	for page := 1; ; page++ {
		var listing struct {
			Artifacts []struct {
				ID      int64  `json:"id"`
				Name    string `json:"name"`
				Expired bool   `json:"expired"`
			} `json:"artifacts"`
		}
		if err := githubRead("actions/runs/"+run+"/artifacts?per_page=100&page="+strconv.Itoa(page), &listing); err != nil {
			return err
		}
		for _, artifact := range listing.Artifacts {
			match := pattern.FindStringSubmatch(artifact.Name)
			if !artifact.Expired && len(match) == 2 && artifact.ID > latest[match[1]] {
				latest[match[1]] = artifact.ID
			}
		}
		if len(listing.Artifacts) < 100 {
			break
		}
	}
	if latest["base"] == 0 || latest["k0s"] == 0 {
		return fmt.Errorf("both profile receipts for this exact build are required")
	}
	if err := workflow.Output("base-id", strconv.FormatInt(latest["base"], 10)); err != nil {
		return err
	}
	return workflow.Output("k0s-id", strconv.FormatInt(latest["k0s"], 10))
}
