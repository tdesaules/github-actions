package pantavisor

import (
	"encoding/json"
	"fmt"
	"github.com/tdesaules/github-actions/shared/workflow"
	"io"
	"net/http"
	"os"
	"regexp"
	"time"
)

func ReadPublished() error {
	if err := Parse(); err != nil {
		return err
	}
	repository := os.Getenv("GITHUB_REPOSITORY")
	if !regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$`).MatchString(repository) {
		return fmt.Errorf("invalid repository")
	}
	token := os.Getenv("INPUT_TOKEN")
	if token == "" {
		return fmt.Errorf("GitHub token required")
	}
	req, err := http.NewRequest("GET", "https://api.github.com/repos/"+repository+"/releases/latest", nil)
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
	if response.StatusCode == 404 {
		return workflow.Output("has-release", false)
	}
	if response.StatusCode != 200 {
		return fmt.Errorf("release lookup failed: HTTP %d", response.StatusCode)
	}
	var release struct {
		Body string `json:"body"`
		Tag  string `json:"tag_name"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 1024*1024)).Decode(&release); err != nil {
		return err
	}
	block := regexp.MustCompile("(?s)```json\\n(.*?)\\n```").FindStringSubmatch(release.Body)
	if len(block) != 2 {
		return fmt.Errorf("release has no immutable publication manifest")
	}
	var manifest struct {
		Version   string `json:"version"`
		Component string `json:"component"`
		Image     string `json:"image"`
	}
	if err := json.Unmarshal([]byte(block[1]), &manifest); err != nil {
		return err
	}
	ref := regexp.MustCompile(`^ghcr\.io/[a-z0-9._/-]+@sha256:[a-f0-9]{64}$`)
	if release.Tag != "v"+manifest.Version || !ref.MatchString(manifest.Component) || (manifest.Image != "" && !ref.MatchString(manifest.Image)) {
		return fmt.Errorf("invalid publication manifest")
	}
	if err := workflow.Output("component", manifest.Component); err != nil {
		return err
	}
	if err := workflow.Output("image", manifest.Image); err != nil {
		return err
	}
	return workflow.Output("has-release", true)
}
