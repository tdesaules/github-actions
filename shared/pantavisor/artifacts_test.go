package pantavisor

import (
	"encoding/json"
	"flag"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestQualificationPaginationAndAttemptSelection(t *testing.T) {
	t.Setenv("GITHUB_REPOSITORY", "fixture/pantavisor-system")
	t.Setenv("GITHUB_RUN_ID", "123")
	t.Setenv("INPUT_TOKEN", "fixture-token")
	output := filepath.Join(t.TempDir(), "outputs")
	os.WriteFile(output, nil, 0600)
	t.Setenv("GITHUB_OUTPUT", output)
	oldTransport := http.DefaultTransport
	oldFlags, oldArgs := flag.CommandLine, os.Args
	t.Cleanup(func() { http.DefaultTransport = oldTransport; flag.CommandLine = oldFlags; os.Args = oldArgs })
	calls := 0
	missing := false
	http.DefaultTransport = roundTripFunc(func(request *http.Request) (*http.Response, error) {
		calls++
		if request.URL.Host != "api.github.com" || request.URL.Query().Get("per_page") != "100" {
			t.Fatal("unexpected artifact request")
		}
		rows := []map[string]any{}
		if request.URL.Query().Get("page") == "1" {
			for i := 0; i < 100; i++ {
				rows = append(rows, map[string]any{"id": i + 1, "name": "unrelated", "expired": false})
			}
			rows[1] = map[string]any{"id": 2, "name": "tested-system-base-55-1", "expired": false}
			rows[98] = map[string]any{"id": 99, "name": "tested-system-k0s-55-9", "expired": true}
			rows[99] = map[string]any{"id": 100, "name": "tested-system-k0s-56-1", "expired": false}
		} else {
			rows = append(rows, map[string]any{"id": 201, "name": "tested-system-base-55-2", "expired": false})
			if !missing {
				rows = append(rows, map[string]any{"id": 202, "name": "tested-system-k0s-55-1", "expired": false})
			}
		}
		data, _ := json.Marshal(map[string]any{"artifacts": rows})
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(string(data))), Header: http.Header{}}, nil
	})
	invoke := func() error {
		flag.CommandLine = flag.NewFlagSet("fixture", flag.ContinueOnError)
		os.Args = []string{"fixture", "--build-artifact-id=55"}
		return QualificationArtifacts()
	}
	if err := invoke(); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(output)
	if calls != 2 || !strings.Contains(string(data), "base-id=201") || !strings.Contains(string(data), "k0s-id=202") {
		t.Fatal("pagination or attempt selection failed", calls, string(data))
	}
	missing = true
	os.WriteFile(output, nil, 0600)
	if err := invoke(); err == nil {
		t.Fatal("missing matching profile accepted")
	}
	data, _ = os.ReadFile(output)
	if len(data) != 0 {
		t.Fatal("partial profile outputs emitted")
	}
}
