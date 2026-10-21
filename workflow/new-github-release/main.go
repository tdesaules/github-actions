package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/tdesaules/github-actions/shared/publication"
	"github.com/tdesaules/github-actions/shared/workflow"
)

func run() error {
	repo := flag.String("repository", workflow.Default("INPUT_REPOSITORY", os.Getenv("GITHUB_REPOSITORY")), "repository")
	tag := flag.String("tag", os.Getenv("INPUT_TAG"), "verified release tag")
	notes := flag.String("notes", workflow.Default("INPUT_NOTES", "CHANGELOG.md"), "release notes file")
	flag.Parse()
	if flag.NArg() != 0 || !publication.SemVer.MatchString(*tag) || !regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$`).MatchString(*repo) {
		return fmt.Errorf("invalid release inputs")
	}
	token := os.Getenv("INPUT_TOKEN")
	if token == "" {
		return fmt.Errorf("token required")
	}
	client := &http.Client{Timeout: 60 * time.Second, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}
	request := func(method, path string, body []byte) (*http.Response, error) {
		req, err := http.NewRequest(method, "https://api.github.com/repos/"+*repo+path, bytes.NewReader(body))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Accept", "application/vnd.github+json")
		req.Header.Set("Content-Type", "application/json")
		return client.Do(req)
	}
	ref, err := request("GET", "/git/ref/tags/"+*tag, nil)
	if err != nil {
		return err
	}
	ref.Body.Close()
	if ref.StatusCode != 200 {
		return fmt.Errorf("release tag must already exist (HTTP %d)", ref.StatusCode)
	}
	existing, err := request("GET", "/releases/tags/"+*tag, nil)
	if err != nil {
		return err
	}
	existing.Body.Close()
	if existing.StatusCode == 200 {
		return workflow.Output("created", false)
	}
	if existing.StatusCode != 404 {
		return fmt.Errorf("GitHub returned HTTP %d", existing.StatusCode)
	}
	body, err := os.ReadFile(*notes)
	if err != nil {
		return err
	}
	data, _ := json.Marshal(map[string]any{"tag_name": *tag, "name": *tag, "body": string(body), "draft": false, "prerelease": strings.Contains(strings.Split(*tag, "+")[0], "-")})
	response, err := request("POST", "/releases", data)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != 201 {
		return fmt.Errorf("release creation returned HTTP %d", response.StatusCode)
	}
	return workflow.Output("created", true)
}
func main() { workflow.Main("new-github-release 1", run) }
