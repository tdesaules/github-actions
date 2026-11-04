package pantavisor

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/tdesaules/github-actions/shared/component"
	"github.com/tdesaules/github-actions/shared/workflow"
)

type Candidate struct {
	Schema        int    `json:"schema"`
	Base          string `json:"base"`
	Revision      string `json:"revision"`
	Version       string `json:"version"`
	RunID         string `json:"run_id"`
	RunAttempt    string `json:"run_attempt,omitempty"`
	ArtifactID    string `json:"artifact_id,omitempty"`
	Project       string `json:"project"`
	ReceiptDigest string `json:"receipt_digest,omitempty"`
}

func git(args ...string) (string, error) {
	out, err := workflow.Git(args...)
	return strings.TrimSpace(out), err
}
func nextVersion(current, previous, messages string) (string, bool, error) {
	stable := regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$`)
	if !stable.MatchString(current) {
		return "", false, fmt.Errorf("VERSION must be stable SemVer")
	}
	if previous == "" {
		return current, true, nil
	}
	last := strings.TrimPrefix(previous, "v")
	if !stable.MatchString(last) {
		return "", false, fmt.Errorf("invalid previous release")
	}
	bump := 0
	if regexp.MustCompile(`(?m)^[a-z]+(?:\([^\n]+\))?!:|^BREAKING[ -]CHANGE:`).MatchString(messages) {
		bump = 3
	} else if regexp.MustCompile(`(?m)^feat(?:\([^\n]+\))?:`).MatchString(messages) {
		bump = 2
	} else if regexp.MustCompile(`(?m)^(fix|perf|refactor)(?:\([^\n]+\))?:`).MatchString(messages) {
		bump = 1
	}
	if bump == 0 {
		return current, false, nil
	}
	parts := strings.Split(last, ".")
	numbers := make([]int, 3)
	for i, v := range parts {
		n, err := strconv.Atoi(v)
		if err != nil {
			return "", false, err
		}
		numbers[i] = n
	}
	at := 3 - bump
	numbers[at]++
	for i := at + 1; i < 3; i++ {
		numbers[i] = 0
	}
	return fmt.Sprintf("%d.%d.%d", numbers[0], numbers[1], numbers[2]), true, nil
}
func NewCandidate() error {
	output, project := Input("output", "artifacts/release"), Input("project", "")
	if err := Parse(); err != nil {
		return err
	}
	if os.Getenv("GITHUB_REF") != "refs/heads/main" {
		return fmt.Errorf("release preparation requires main")
	}
	if *project != "core" && *project != "k0s" && *project != "system" {
		return fmt.Errorf("invalid project")
	}
	if err := Project("."); err != nil {
		return err
	}
	dirty, err := git("status", "--porcelain")
	if err != nil {
		return err
	}
	if dirty != "" {
		return fmt.Errorf("release preparation requires a clean checkout")
	}
	base, err := git("rev-parse", "HEAD")
	if err != nil {
		return err
	}
	version, err := os.ReadFile("VERSION")
	if err != nil {
		return err
	}
	tags, err := git("tag", "--merged", "HEAD", "--sort=-version:refname", "--list", "v*")
	if err != nil {
		return err
	}
	previous := ""
	for _, tag := range strings.Fields(tags) {
		if regexp.MustCompile(`^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$`).MatchString(tag) {
			previous = tag
			break
		}
	}
	rangeRef := "HEAD"
	if previous != "" {
		rangeRef = previous + "..HEAD"
	}
	messages, err := git("log", "--format=%B", rangeRef, "--")
	if err != nil {
		return err
	}
	next, changed, err := nextVersion(strings.TrimSpace(string(version)), previous, messages)
	if err != nil {
		return err
	}
	if err := workflow.Output("changed", changed); err != nil {
		return err
	}
	if !changed {
		return nil
	}
	if _, err := git("checkout", "--detach", "HEAD"); err != nil {
		return err
	}
	if err := os.WriteFile("VERSION", []byte(next+"\n"), 0644); err != nil {
		return err
	}
	subjects, err := git("log", "--format=- %s (%h)", rangeRef, "--")
	if err != nil {
		return err
	}
	old, _ := os.ReadFile("CHANGELOG.md")
	notes := "# Changelog\n\n## " + next + "\n\n" + subjects + "\n\n" + strings.TrimPrefix(string(old), "# Changelog\n")
	if err := os.WriteFile("CHANGELOG.md", []byte(notes), 0644); err != nil {
		return err
	}
	if _, err := git("add", "--", "VERSION", "CHANGELOG.md"); err != nil {
		return err
	}
	if _, err := git("-c", "user.name=Pantavisor Release", "-c", "user.email=release@users.noreply.github.com", "commit", "-m", "chore(release): "+next); err != nil {
		return err
	}
	revision, err := git("rev-parse", "HEAD")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(*output, 0755); err != nil {
		return err
	}
	if _, err := git("update-ref", "refs/heads/pantavisor-candidate", revision); err != nil {
		return err
	}
	if _, err := git("bundle", "create", filepath.Join(*output, "candidate.bundle"), "refs/heads/pantavisor-candidate", "^"+base); err != nil {
		return err
	}
	candidate := Candidate{Schema: 1, Base: base, Revision: revision, Version: next, RunID: os.Getenv("GITHUB_RUN_ID"), Project: *project}
	if err := WriteJSON(filepath.Join(*output, "candidate.json"), candidate); err != nil {
		return err
	}
	if err := workflow.Output("revision", revision); err != nil {
		return err
	}
	return workflow.Output("version", next)
}
func ImportCandidate() error {
	directory := Input("directory", "artifacts/release")
	if err := Parse(); err != nil {
		return err
	}
	if err := Project("."); err != nil {
		return err
	}
	var candidate Candidate
	if err := ReadJSON(filepath.Join(*directory, "candidate.json"), &candidate); err != nil {
		return err
	}
	head, err := git("rev-parse", "HEAD")
	if err != nil {
		return err
	}
	if candidate.Schema != 1 || head != candidate.Base {
		return fmt.Errorf("candidate does not extend this checkout")
	}
	bundle := filepath.Join(*directory, "candidate.bundle")
	if _, err := git("bundle", "verify", bundle); err != nil {
		return err
	}
	if _, err := git("fetch", bundle, "refs/heads/pantavisor-candidate"); err != nil {
		return err
	}
	fetched, err := git("rev-parse", "FETCH_HEAD")
	if err != nil || fetched != candidate.Revision {
		return fmt.Errorf("candidate revision mismatch")
	}
	if _, err := git("checkout", "--detach", candidate.Revision); err != nil {
		return err
	}
	version, err := os.ReadFile("VERSION")
	if err != nil || strings.TrimSpace(string(version)) != candidate.Version {
		return fmt.Errorf("candidate VERSION mismatch")
	}
	return workflow.Output("revision", candidate.Revision)
}
func FinalizeCandidate() error {
	directory, receipt := Input("directory", "artifacts/release"), Input("receipt", "artifacts/qualified-receipt.json")
	artifactID := Input("artifact-id", "")
	if err := Parse(); err != nil {
		return err
	}
	if err := Project("."); err != nil {
		return err
	}
	var candidate Candidate
	if err := ReadJSON(filepath.Join(*directory, "candidate.json"), &candidate); err != nil {
		return err
	}
	candidate.ArtifactID = *artifactID
	candidate.RunAttempt = os.Getenv("GITHUB_RUN_ATTEMPT")
	if !regexp.MustCompile(`^[1-9][0-9]*$`).MatchString(candidate.ArtifactID) || !regexp.MustCompile(`^[1-9][0-9]*$`).MatchString(candidate.RunAttempt) {
		return fmt.Errorf("qualified artifact ID and preparation attempt are required")
	}
	head, err := git("rev-parse", "HEAD")
	if err != nil || head != candidate.Revision {
		return fmt.Errorf("checkout differs from candidate")
	}
	qualified, err := CheckReceipt("dist", *receipt, candidate.Revision, true)
	if err != nil {
		return err
	}
	if qualified.Project != candidate.Project {
		return fmt.Errorf("qualified project differs from candidate")
	}
	candidate.ReceiptDigest, err = component.Hash(*receipt)
	if err != nil {
		return err
	}
	repository := os.Getenv("GITHUB_REPOSITORY")
	token := os.Getenv("INPUT_TOKEN")
	if !regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$`).MatchString(repository) || token == "" {
		return fmt.Errorf("repository and release App token required")
	}
	// Credentials exist only in the Git child environment, never in argv/config.
	remote := "https://github.com/" + repository + ".git"
	command := func(args ...string) (string, error) {
		cmd := exec.Command("git", args...)
		root, _ := os.Getwd()
		cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "GIT_CONFIG_COUNT=2", "GIT_CONFIG_KEY_0=safe.directory", "GIT_CONFIG_VALUE_0="+root, "GIT_CONFIG_KEY_1=http.https://github.com/.extraheader", "GIT_CONFIG_VALUE_1=AUTHORIZATION: basic "+base64.StdEncoding.EncodeToString([]byte("x-access-token:"+token)))
		cmd.Stderr = os.Stderr
		out, e := cmd.Output()
		return strings.TrimSpace(string(out)), e
	}
	refs, err := command("ls-remote", remote, "refs/heads/main", "refs/tags/v"+candidate.Version)
	if err != nil {
		return err
	}
	for _, line := range strings.Split(refs, "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 {
			return fmt.Errorf("invalid remote refs")
		}
		if fields[1] != "refs/heads/main" || fields[0] != candidate.Base {
			return fmt.Errorf("main advanced or release tag already exists")
		}
	}
	if !strings.Contains(refs, "refs/heads/main") {
		return fmt.Errorf("remote main missing")
	}
	data, _ := json.Marshal(candidate)
	if _, err := git("-c", "user.name=Pantavisor Release", "-c", "user.email=release@users.noreply.github.com", "tag", "-a", "v"+candidate.Version, "-m", string(data), candidate.Revision); err != nil {
		return err
	}
	_, err = command("push", "--atomic", remote, candidate.Revision+":refs/heads/main", "refs/tags/v"+candidate.Version)
	return err
}
func ReadRelease() error {
	if err := Parse(); err != nil {
		return err
	}
	if err := Project("."); err != nil {
		return err
	}
	ref := os.Getenv("GITHUB_REF")
	if !regexp.MustCompile(`^refs/tags/v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$`).MatchString(ref) {
		return fmt.Errorf("expected Pantavisor release tag")
	}
	kind, err := git("cat-file", "-t", ref)
	if err != nil || kind != "tag" {
		return fmt.Errorf("release requires an annotated provenance tag")
	}
	text, err := git("for-each-ref", "--format=%(contents:subject)", ref)
	if err != nil {
		return err
	}
	var candidate Candidate
	if err := json.Unmarshal([]byte(text), &candidate); err != nil {
		return fmt.Errorf("tag lacks qualification provenance: %w", err)
	}
	head, err := git("rev-parse", "HEAD")
	if err != nil {
		return err
	}
	if candidate.Schema != 1 || candidate.Revision != head || ref != "refs/tags/v"+candidate.Version || !regexp.MustCompile(`^[1-9][0-9]*$`).MatchString(candidate.RunID) || !regexp.MustCompile(`^[1-9][0-9]*$`).MatchString(candidate.RunAttempt) || !regexp.MustCompile(`^[1-9][0-9]*$`).MatchString(candidate.ArtifactID) {
		return fmt.Errorf("tag identity mismatch")
	}
	if !regexp.MustCompile(`^[a-f0-9]{64}$`).MatchString(candidate.ReceiptDigest) || (candidate.Project != "core" && candidate.Project != "k0s" && candidate.Project != "system") {
		return fmt.Errorf("tag qualification metadata is incomplete")
	}
	version, err := os.ReadFile("VERSION")
	if err != nil || strings.TrimSpace(string(version)) != candidate.Version {
		return fmt.Errorf("tag VERSION differs from the checked-out source")
	}
	if _, err := git("merge-base", "--is-ancestor", head, "origin/main"); err != nil {
		return fmt.Errorf("tag must belong to main")
	}
	token := os.Getenv("INPUT_TOKEN")
	repository := os.Getenv("GITHUB_REPOSITORY")
	client := http.Client{Timeout: 30 * time.Second}
	for attempt := 0; attempt < 30; attempt++ {
		req, err := http.NewRequest("GET", "https://api.github.com/repos/"+repository+"/actions/runs/"+candidate.RunID+"/attempts/"+candidate.RunAttempt, nil)
		if err != nil {
			return err
		}
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Accept", "application/vnd.github+json")
		response, err := client.Do(req)
		if err != nil {
			return err
		}
		var run struct {
			Status     string `json:"status"`
			Conclusion string `json:"conclusion"`
			SHA        string `json:"head_sha"`
			Path       string `json:"path"`
			Event      string `json:"event"`
			Branch     string `json:"head_branch"`
		}
		err = json.NewDecoder(response.Body).Decode(&run)
		response.Body.Close()
		if err != nil || response.StatusCode != 200 {
			return fmt.Errorf("cannot verify preparation run")
		}
		if run.SHA != candidate.Base || run.Path != ".github/workflows/main.yml" || run.Event != "workflow_dispatch" || run.Branch != "main" {
			return fmt.Errorf("preparation run identity differs")
		}
		if run.Status == "completed" {
			if run.Conclusion != "success" {
				return fmt.Errorf("preparation run did not succeed")
			}
			var artifact struct {
				Expired     bool `json:"expired"`
				WorkflowRun struct {
					ID int64 `json:"id"`
				} `json:"workflow_run"`
			}
			if err := githubRead("actions/artifacts/"+candidate.ArtifactID, &artifact); err != nil {
				return err
			}
			if artifact.Expired || strconv.FormatInt(artifact.WorkflowRun.ID, 10) != candidate.RunID {
				return fmt.Errorf("qualified artifact expired or belongs to another run")
			}
			if err := workflow.Output("artifact-id", candidate.ArtifactID); err != nil {
				return err
			}
			if err := workflow.Output("run-id", candidate.RunID); err != nil {
				return err
			}
			if err := workflow.Output("receipt-digest", candidate.ReceiptDigest); err != nil {
				return err
			}
			return workflow.Output("version", candidate.Version)
		}
		time.Sleep(10 * time.Second)
	}
	return fmt.Errorf("preparation run is not complete")
}
