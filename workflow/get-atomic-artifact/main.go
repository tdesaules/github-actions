package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const api = "https://api.github.com"

var errArtifactMissing = errors.New("no matching artifact in current workflow run")

type options struct {
	Repository, RunID, SHA, Attempt, Kind, Architecture string
	ExpectedID                                          string
}

type artifact struct {
	ID      int64  `json:"id"`
	Name    string `json:"name"`
	Expired bool   `json:"expired"`
	Run     *struct {
		ID      int64  `json:"id"`
		HeadSHA string `json:"head_sha"`
	} `json:"workflow_run"`
}

type selected struct {
	artifact
	Attempt int64
}

func positive(s string) (int64, error) {
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil || n < 1 || strconv.FormatInt(n, 10) != s {
		return 0, errors.New("expected canonical positive integer")
	}
	return n, nil
}

func validate(o options, env func(string) string) error {
	if !regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$`).MatchString(o.Repository) ||
		!regexp.MustCompile(`^[a-f0-9]{40}$`).MatchString(o.SHA) {
		return errors.New("invalid repository or workflow SHA")
	}
	for _, part := range strings.Split(o.Repository, "/") {
		if part == "." || part == ".." {
			return errors.New("invalid repository")
		}
	}
	if _, err := positive(o.RunID); err != nil {
		return errors.New("invalid run ID")
	}
	if _, err := positive(o.Attempt); err != nil {
		return errors.New("invalid maximum attempt")
	}
	if o.ExpectedID != "" {
		if _, err := positive(o.ExpectedID); err != nil {
			return errors.New("invalid expected producer artifact ID")
		}
	}
	if o.Kind != "candidate" && o.Kind != "qualified" {
		return errors.New("kind must be candidate or qualified")
	}
	if o.Architecture != "amd64" && o.Architecture != "arm64" {
		return errors.New("architecture must be amd64 or arm64")
	}
	for name, expected := range map[string]string{
		"GITHUB_REPOSITORY": o.Repository, "GITHUB_RUN_ID": o.RunID,
		"GITHUB_SHA": o.SHA, "GITHUB_RUN_ATTEMPT": o.Attempt,
	} {
		if env(name) != expected {
			return fmt.Errorf("%s does not match explicit current-workflow input", name)
		}
	}
	if origin := env("GITHUB_API_URL"); origin != "" && origin != api {
		return errors.New("only the GitHub.com API is supported")
	}
	if strings.TrimSpace(env("INPUT_TOKEN")) == "" {
		return errors.New("repository-scoped actions read token required")
	}
	return nil
}

func get(ctx context.Context, client *http.Client, token, path string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, api+path, nil)
	if err != nil {
		return errors.New("cannot construct GitHub request")
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("User-Agent", "get-atomic-artifact")
	resp, err := client.Do(req)
	if err != nil {
		return errors.New("GitHub request failed")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("GitHub request returned HTTP %d", resp.StatusCode)
	}
	// Do not log response bodies, authorization headers or transport diagnostics.
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8*1024*1024+1))
	if err != nil || len(body) > 8*1024*1024 || json.Unmarshal(body, out) != nil {
		return errors.New("invalid or oversized GitHub response")
	}
	return nil
}

func selectArtifact(ctx context.Context, client *http.Client, token string, o options) (selected, error) {
	var run struct {
		ID         int64  `json:"id"`
		HeadSHA    string `json:"head_sha"`
		RunAttempt int64  `json:"run_attempt"`
		Repository struct {
			FullName string `json:"full_name"`
		} `json:"repository"`
	}
	base := "/repos/" + o.Repository + "/actions/runs/" + o.RunID
	if err := get(ctx, client, token, base, &run); err != nil {
		return selected{}, err
	}
	runID, _ := positive(o.RunID)
	max, _ := positive(o.Attempt)
	expectedID, _ := positive(o.ExpectedID)
	if run.ID != runID || run.HeadSHA != o.SHA || run.Repository.FullName != o.Repository || run.RunAttempt < max {
		return selected{}, errors.New("API workflow run identity does not match current workflow")
	}
	pattern := regexp.MustCompile(`^` + o.Kind + `-os-` + o.Architecture + `-` + o.RunID + `-([1-9][0-9]*)$`)
	best := selected{}
	expectedSeen := false
	seen := map[int64]bool{}
	total, count := -1, 0
	for page := 1; page <= 1000; page++ {
		var list struct {
			Total     *int       `json:"total_count"`
			Artifacts []artifact `json:"artifacts"`
		}
		if err := get(ctx, client, token, fmt.Sprintf("%s/artifacts?per_page=100&page=%d", base, page), &list); err != nil {
			return selected{}, err
		}
		if list.Total == nil || *list.Total < 0 || *list.Total > 100000 || len(list.Artifacts) > 100 {
			return selected{}, errors.New("invalid artifact pagination")
		}
		if total < 0 {
			total = *list.Total
		}
		if total != *list.Total {
			return selected{}, errors.New("artifact listing changed during pagination")
		}
		count += len(list.Artifacts)
		if count > total || (len(list.Artifacts) == 0 && count < total) {
			return selected{}, errors.New("incomplete artifact pagination")
		}
		for _, a := range list.Artifacts {
			m := pattern.FindStringSubmatch(a.Name)
			if m == nil {
				if expectedID != 0 && a.ID == expectedID {
					return selected{}, errors.New("expected producer artifact has the wrong identity")
				}
				continue
			}
			attempt, err := positive(m[1])
			if err != nil {
				return selected{}, errors.New("invalid artifact attempt")
			}
			if attempt > max {
				if expectedID != 0 && a.ID == expectedID {
					return selected{}, errors.New("expected producer artifact exceeds current attempt")
				}
				continue
			}
			if seen[attempt] {
				return selected{}, errors.New("ambiguous duplicate artifact attempt")
			}
			seen[attempt] = true
			if a.ID < 1 || (a.Run != nil && (a.Run.ID != runID || a.Run.HeadSHA != o.SHA)) {
				return selected{}, errors.New("artifact workflow identity mismatch")
			}
			if a.ID == expectedID {
				expectedSeen = true
			}
			if attempt > best.Attempt {
				best = selected{a, attempt}
			}
		}
		if count == total {
			if best.ID == 0 {
				return selected{}, errArtifactMissing
			}
			if best.Expired {
				return selected{}, errors.New("newest matching artifact expired; rerun its producer")
			}
			if expectedID != 0 {
				if !expectedSeen {
					// A successful upload's ID is not visible yet. Do not return
					// an older producer while waiting for that known new upload.
					return selected{}, errArtifactMissing
				}
				if best.ID != expectedID {
					return selected{}, errors.New("expected producer artifact is not the newest eligible artifact")
				}
			}
			return best, nil
		}
	}
	return selected{}, errors.New("artifact pagination limit exceeded")
}

func waitForVisibility(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// Opt-in retries require an artifact ID emitted by a successful producer.
// Each lookup revalidates identity and paginates from scratch. Only absent
// artifacts retry: expiry, duplicates, HTTP errors and identity failures do not.
func selectVisibleArtifact(ctx context.Context, client *http.Client, token string, o options, wait func(context.Context, time.Duration) error) (selected, error) {
	for lookup := 0; lookup < 6; lookup++ {
		if err := ctx.Err(); err != nil {
			return selected{}, err
		}
		result, err := selectArtifact(ctx, client, token, o)
		if err == nil || !errors.Is(err, errArtifactMissing) || o.ExpectedID == "" {
			return result, err
		}
		if lookup == 5 {
			return selected{}, fmt.Errorf("uploaded artifact not visible after 6 lookups: %w", err)
		}
		if err := wait(ctx, time.Second<<lookup); err != nil {
			return selected{}, err
		}
	}
	panic("unreachable visibility loop")
}

func run(args []string) error {
	if len(args) == 1 && args[0] == "--tool-version" {
		fmt.Println("get-atomic-artifact 1")
		return nil
	}
	var o options
	f := flag.NewFlagSet("get-atomic-artifact", flag.ContinueOnError)
	f.SetOutput(io.Discard)
	f.StringVar(&o.Repository, "repository", "", "current repository")
	f.StringVar(&o.RunID, "run-id", "", "current workflow run")
	f.StringVar(&o.SHA, "workflow-sha", "", "current workflow SHA")
	f.StringVar(&o.Attempt, "max-attempt", "", "current workflow attempt")
	f.StringVar(&o.Kind, "kind", "", "candidate or qualified")
	f.StringVar(&o.Architecture, "architecture", "", "amd64 or arm64")
	f.StringVar(&o.ExpectedID, "expected-artifact-id", "", "artifact ID returned by a successful producer; enables bounded visibility retries")
	if err := f.Parse(args); err != nil || f.NArg() != 0 {
		return errors.New("invalid action arguments")
	}
	if err := validate(o, os.Getenv); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	client := &http.Client{
		Timeout: 20 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return errors.New("redirects forbidden")
		},
	}
	result, err := selectVisibleArtifact(ctx, client, os.Getenv("INPUT_TOKEN"), o, waitForVisibility)
	if err != nil {
		return err
	}
	file, err := os.OpenFile(os.Getenv("GITHUB_OUTPUT"), os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return errors.New("cannot open GitHub output file")
	}
	_, err = fmt.Fprintf(file, "artifact-name=%s\nartifact-id=%d\nproducer-attempt=%d\n", result.Name, result.ID, result.Attempt)
	closeErr := file.Close()
	if err != nil || closeErr != nil {
		return errors.New("cannot write GitHub outputs")
	}
	return nil
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
