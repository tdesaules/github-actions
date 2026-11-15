package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
)

type transport func(*http.Request) (*http.Response, error)

func (f transport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func fixture() options {
	return options{Repository: "owner/repo", RunID: "42", SHA: strings.Repeat("a", 40), Attempt: "3", Kind: "candidate", Architecture: "amd64"}
}

func envFor(o options) map[string]string {
	return map[string]string{
		"GITHUB_REPOSITORY": o.Repository, "GITHUB_RUN_ID": o.RunID,
		"GITHUB_SHA": o.SHA, "GITHUB_RUN_ATTEMPT": o.Attempt,
		"GITHUB_API_URL": api, "INPUT_TOKEN": "secret-token",
	}
}

func item(attempt int, expired bool) artifact {
	return artifact{ID: int64(100 + attempt), Name: fmt.Sprintf("candidate-os-amd64-42-%d", attempt), Expired: expired}
}

func mock(t *testing.T, o options, pages [][]artifact, changeRun func(map[string]any)) (*http.Client, *int) {
	t.Helper()
	calls := 0
	run := map[string]any{"id": 42, "head_sha": o.SHA, "run_attempt": 3, "repository": map[string]any{"full_name": o.Repository}}
	if changeRun != nil {
		changeRun(run)
	}
	total := 0
	for _, p := range pages {
		total += len(p)
	}
	return &http.Client{Transport: transport(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.URL.Scheme != "https" || r.URL.Host != "api.github.com" || r.Method != "GET" {
			t.Fatalf("unexpected destination: %s", r.URL)
		}
		if r.Header.Get("Authorization") != "Bearer secret-token" {
			t.Fatal("missing auth")
		}
		var body any = run
		if calls == 1 {
			if r.URL.Path != "/repos/owner/repo/actions/runs/42" {
				t.Fatal("wrong run endpoint")
			}
		} else {
			if calls-2 >= len(pages) || r.URL.Query().Get("page") != fmt.Sprint(calls-1) || r.URL.Query().Get("per_page") != "100" {
				t.Fatal("wrong pagination")
			}
			body = map[string]any{"total_count": total, "artifacts": pages[calls-2]}
		}
		data, _ := json.Marshal(body)
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(string(data))), Header: http.Header{}}, nil
	})}, &calls
}

func TestSelection(t *testing.T) {
	for _, tc := range []struct {
		name      string
		items     []artifact
		want      int64
		errorText string
	}{
		{"current", []artifact{item(1, false), item(3, false)}, 3, ""},
		{"earlier successful producer", []artifact{item(1, false), item(2, false)}, 2, ""},
		{"ignore future and other arch run kind", []artifact{
			item(2, false), item(4, false),
			{ID: 500, Name: "candidate-os-arm64-42-3"},
			{ID: 501, Name: "candidate-os-amd64-43-3"},
			{ID: 502, Name: "qualified-os-amd64-42-3"},
			{ID: 503, Name: "candidate-os-amd64-42-03"},
		}, 2, ""},
		{"old expired does not hide current", []artifact{item(1, true), item(3, false)}, 3, ""},
		{"newest expired must not fall back", []artifact{item(1, false), item(2, true)}, 0, "newest matching artifact expired"},
		{"duplicate attempt", []artifact{item(2, false), {ID: 999, Name: item(2, false).Name}}, 0, "ambiguous"},
		{"no artifacts", nil, 0, "no matching"},
		{"only future", []artifact{item(4, false)}, 0, "no matching"},
		{"invalid id", []artifact{{Name: item(1, false).Name}}, 0, "identity mismatch"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			o := fixture()
			client, _ := mock(t, o, [][]artifact{tc.items}, nil)
			got, err := selectArtifact(context.Background(), client, "secret-token", o)
			if tc.errorText != "" {
				if err == nil || !strings.Contains(err.Error(), tc.errorText) {
					t.Fatalf("wanted %q, got %v", tc.errorText, err)
				}
			} else if err != nil || got.Attempt != tc.want || got.ID != 100+tc.want {
				t.Fatalf("got %+v %v", got, err)
			}
		})
	}
}

func TestPaginationAndQualifiedARM64(t *testing.T) {
	o := fixture()
	o.Kind, o.Architecture = "qualified", "arm64"
	first := make([]artifact, 100)
	for i := range first {
		first[i] = artifact{ID: int64(i + 1), Name: fmt.Sprintf("unrelated-%d", i)}
	}
	client, calls := mock(t, o, [][]artifact{first, {{ID: 999, Name: "qualified-os-arm64-42-2"}}}, nil)
	got, err := selectArtifact(context.Background(), client, "secret-token", o)
	if err != nil || got.ID != 999 || got.Attempt != 2 || *calls != 3 {
		t.Fatalf("pagination failed: %+v %v calls=%d", got, err, *calls)
	}
	client, _ = mock(t, fixture(), [][]artifact{{item(2, false)}, {item(2, false)}}, nil)
	if _, err = selectArtifact(context.Background(), client, "secret-token", fixture()); err == nil || !strings.Contains(err.Error(), "ambiguous") {
		t.Fatal("duplicate across pages accepted", err)
	}
}

func TestWrongRunIdentity(t *testing.T) {
	for _, field := range []string{"id", "head_sha", "repository", "run_attempt"} {
		t.Run(field, func(t *testing.T) {
			client, calls := mock(t, fixture(), nil, func(r map[string]any) {
				switch field {
				case "id":
					r[field] = 43
				case "head_sha":
					r[field] = strings.Repeat("b", 40)
				case "repository":
					r[field] = map[string]string{"full_name": "other/repo"}
				case "run_attempt":
					r[field] = 2
				}
			})
			if _, err := selectArtifact(context.Background(), client, "secret-token", fixture()); err == nil || *calls != 1 {
				t.Fatal("wrong run accepted")
			}
		})
	}
	a := item(3, false)
	if err := json.Unmarshal([]byte(`{"workflow_run":{"id":43,"head_sha":"wrong"}}`), &a); err != nil {
		t.Fatal(err)
	}
	client, _ := mock(t, fixture(), [][]artifact{{a}}, nil)
	if _, err := selectArtifact(context.Background(), client, "secret-token", fixture()); err == nil {
		t.Fatal("foreign artifact workflow accepted")
	}
}

func TestContextAndTokenValidation(t *testing.T) {
	o := fixture()
	env := envFor(o)
	if err := validate(o, func(k string) string { return env[k] }); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"GITHUB_REPOSITORY", "GITHUB_RUN_ID", "GITHUB_SHA", "GITHUB_RUN_ATTEMPT", "INPUT_TOKEN"} {
		t.Run(key, func(t *testing.T) {
			env := envFor(o)
			delete(env, key)
			if err := validate(o, func(k string) string { return env[k] }); err == nil {
				t.Fatal("missing context accepted")
			}
		})
	}
	for _, mutate := range []func(*options){
		func(o *options) { o.Repository = "../repo" },
		func(o *options) { o.RunID = "042" },
		func(o *options) { o.Attempt = "0" },
		func(o *options) { o.SHA = "main" },
		func(o *options) { o.Kind = "development" },
		func(o *options) { o.Architecture = "x86_64" },
	} {
		bad := o
		mutate(&bad)
		env := envFor(bad)
		if err := validate(bad, func(k string) string { return env[k] }); err == nil {
			t.Fatalf("accepted %+v", bad)
		}
	}
	env["GITHUB_API_URL"] = "https://example.com"
	if err := validate(o, func(k string) string { return env[k] }); err == nil {
		t.Fatal("foreign API accepted")
	}
}

func TestSafeFailures(t *testing.T) {
	for _, tc := range []struct {
		status  int
		body    string
		network bool
	}{
		{403, "secret-token private server detail", false},
		{200, "secret-token not JSON", false},
		{302, "redirect secret-token", false},
		{0, "", true},
	} {
		client := &http.Client{Transport: transport(func(*http.Request) (*http.Response, error) {
			if tc.network {
				return nil, errors.New("secret-token network diagnostic")
			}
			return &http.Response{StatusCode: tc.status, Body: io.NopCloser(strings.NewReader(tc.body))}, nil
		})}
		_, err := selectArtifact(context.Background(), client, "secret-token", fixture())
		if err == nil || strings.Contains(err.Error(), "secret-token") {
			t.Fatal("unsafe failure", err)
		}
	}
}
