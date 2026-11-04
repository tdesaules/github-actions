package pantavisor

import (
	"encoding/json"
	"flag"
	"fmt"
	"github.com/tdesaules/github-actions/shared/component"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) { return f(request) }

func TestGitHubReleaseCreationRetryAndConflict(t *testing.T) {
	t.Chdir(t.TempDir())
	os.Mkdir("dist", 0755)
	os.WriteFile("dist/payload", []byte("qualified bytes"), 0644)
	files, err := Inventory("dist")
	if err != nil {
		t.Fatal(err)
	}
	revision := strings.Repeat("a", 40)
	if err := WriteJSON("artifacts/qualified-receipt.json", Receipt{Schema: 1, Project: "core", Revision: revision, Version: "0.1.0", Tested: true, Files: files}); err != nil {
		t.Fatal(err)
	}
	digest, err := component.Hash("artifacts/qualified-receipt.json")
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("GITHUB_SHA", revision)
	t.Setenv("GITHUB_REF", "refs/tags/v0.1.0")
	t.Setenv("GITHUB_REPOSITORY", "fixture/pantavisor-core")
	t.Setenv("INPUT_TOKEN", "fixture-token")
	originalTransport := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = originalTransport })
	mode, body := "new", ""
	posts, calls := 0, 0
	http.DefaultTransport = roundTripFunc(func(request *http.Request) (*http.Response, error) {
		calls++
		if request.URL.Host != "api.github.com" || request.Header.Get("Authorization") != "Bearer fixture-token" {
			return nil, fmt.Errorf("unexpected HTTP request")
		}
		status, payload := 200, map[string]any{}
		switch request.Method {
		case "GET":
			switch mode {
			case "new":
				status = 404
			case "existing":
				payload = map[string]any{"body": body, "draft": false}
			case "conflict":
				payload = map[string]any{"body": "different release", "draft": false}
			default:
				status = 403
			}
		case "POST":
			posts++
			var record map[string]any
			if err := json.NewDecoder(request.Body).Decode(&record); err != nil {
				return nil, err
			}
			if record["tag_name"] != "v0.1.0" || record["target_commitish"] != revision {
				return nil, fmt.Errorf("incorrect release identity")
			}
			body = record["body"].(string)
			status = 201
			payload = map[string]any{"id": 1}
		default:
			return nil, fmt.Errorf("unexpected HTTP method")
		}
		data, _ := json.Marshal(payload)
		return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(string(data))), Header: http.Header{}}, nil
	})
	originalFlags, originalArgs := flag.CommandLine, os.Args
	t.Cleanup(func() { flag.CommandLine = originalFlags; os.Args = originalArgs })
	invoke := func() error {
		flag.CommandLine = flag.NewFlagSet("fixture", flag.ContinueOnError)
		os.Args = []string{"fixture", "--receipt-digest=" + digest, "--component=ghcr.io/fixture/component@sha256:" + strings.Repeat("b", 64)}
		return NewGitHubRelease()
	}
	if err := invoke(); err != nil {
		t.Fatal(err)
	}
	if posts != 1 || body == "" {
		t.Fatal("release not created")
	}
	mode = "existing"
	if err := invoke(); err != nil {
		t.Fatal(err)
	}
	if posts != 1 {
		t.Fatal("retry created another release")
	}
	mode = "conflict"
	if err := invoke(); err == nil {
		t.Fatal("conflicting release overwritten")
	}
	mode = "forbidden"
	if err := invoke(); err == nil {
		t.Fatal("permission error treated as missing release")
	}
	before := calls
	t.Setenv("GITHUB_REF", "refs/heads/main")
	if err := invoke(); err == nil || calls != before {
		t.Fatal("branch publication reached GitHub")
	}
}
