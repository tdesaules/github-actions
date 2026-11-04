package pantavisor

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestReleaseUsesExactAttemptAndArtifact(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("GIT_CONFIG_GLOBAL", "/dev/null")
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("GIT_CONFIG_COUNT", "0")
	runGit := func(args ...string) string {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=Fixture", "GIT_AUTHOR_EMAIL=fixture@example.invalid", "GIT_COMMITTER_NAME=Fixture", "GIT_COMMITTER_EMAIL=fixture@example.invalid")
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %s %v", args, out, err)
		}
		return strings.TrimSpace(string(out))
	}
	runGit("init", "-b", "main")
	os.WriteFile("VERSION", []byte("0.1.0\n"), 0644)
	runGit("add", "VERSION")
	runGit("commit", "-m", "feat: initialize")
	head := runGit("rev-parse", "HEAD")
	runGit("update-ref", "refs/remotes/origin/main", head)
	candidate := Candidate{Schema: 1, Base: strings.Repeat("b", 40), Revision: head, Version: "0.1.0", Project: "core", RunID: "123", RunAttempt: "1", ArtifactID: "42", ReceiptDigest: strings.Repeat("c", 64)}
	annotation, _ := json.Marshal(candidate)
	runGit("tag", "-a", "v0.1.0", "-m", string(annotation))
	output := filepath.Join(t.TempDir(), "outputs")
	os.WriteFile(output, nil, 0600)
	t.Setenv("GITHUB_OUTPUT", output)
	t.Setenv("GITHUB_REF", "refs/tags/v0.1.0")
	t.Setenv("GITHUB_REPOSITORY", "fixture/pantavisor-core")
	t.Setenv("INPUT_TOKEN", "fixture-token")
	transport, flags, args := http.DefaultTransport, flag.CommandLine, os.Args
	t.Cleanup(func() { http.DefaultTransport = transport; flag.CommandLine = flags; os.Args = args })
	expired := false
	requests := 0
	http.DefaultTransport = roundTripFunc(func(request *http.Request) (*http.Response, error) {
		requests++
		var data any
		switch request.URL.Path {
		case "/repos/fixture/pantavisor-core/actions/runs/123/attempts/1":
			data = map[string]any{"head_sha": candidate.Base, "head_branch": "main", "path": ".github/workflows/main.yml", "event": "workflow_dispatch", "status": "completed", "conclusion": "success"}
		case "/repos/fixture/pantavisor-core/actions/artifacts/42":
			data = map[string]any{"expired": expired, "workflow_run": map[string]any{"id": 123}}
		default:
			return nil, fmt.Errorf("unexpected or unscoped source request: %s", request.URL.Path)
		}
		body, _ := json.Marshal(data)
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(string(body))), Header: http.Header{}}, nil
	})
	invoke := func() error {
		flag.CommandLine = flag.NewFlagSet("fixture", flag.ContinueOnError)
		os.Args = []string{"fixture"}
		return ReadRelease()
	}
	if err := invoke(); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(output)
	if requests != 2 || !strings.Contains(string(data), "artifact-id=42") {
		t.Fatal("wrong release evidence selected", requests, string(data))
	}
	expired = true
	os.WriteFile(output, nil, 0600)
	if err := invoke(); err == nil {
		t.Fatal("expired evidence accepted")
	}
	data, _ = os.ReadFile(output)
	if len(data) != 0 {
		t.Fatal("expired evidence emitted outputs")
	}
}
