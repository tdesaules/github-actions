package binaryrelease

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestBindAttemptScopedArtifactsAcrossPages(t *testing.T) {
	dir, m := qualifiedFixture(t)
	q, e := Verify(dir, m.SourceSHA, m.Version, m.RunID, m.RunAttempt)
	if e != nil {
		t.Fatal(e)
	}
	bindings := m.Artifacts
	includeCorrect := false
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/o/r/actions/runs/123/attempts/2":
			_ = json.NewEncoder(w).Encode(Run{ID: 123, HeadSHA: m.SourceSHA, HeadBranch: "main", Event: "workflow_dispatch", Status: "in_progress", Attempt: 2})
		case "/repos/o/r/actions/runs/123/artifacts":
			arts := []Artifact{}
			if r.URL.Query().Get("page") == "1" {
				for i := 0; i < 100; i++ {
					ar := Artifact{ID: int64(i + 1), Name: fmt.Sprintf("unrelated-%d", i)}
					if i < 2 {
						ar.Name = "qualified-" + q[i].Arch + "-123-attempt-1"
						ar.Digest = bindings[i].Digest
						ar.WorkflowRun.ID = 123
						ar.WorkflowRun.HeadSHA = m.SourceSHA
					}
					arts = append(arts, ar)
				}
			} else if includeCorrect {
				for i, v := range q {
					ar := Artifact{ID: int64(101 + i), Name: "qualified-" + v.Arch + "-123-attempt-2", Digest: bindings[i].Digest}
					ar.WorkflowRun.ID = 123
					ar.WorkflowRun.HeadSHA = m.SourceSHA
					arts = append(arts, ar)
				}
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"artifacts": arts})
		default:
			t.Errorf("unexpected endpoint %s", r.URL.Path)
			w.WriteHeader(500)
		}
	}))
	defer s.Close()
	a := API{s.URL, "o/r", "test", s.Client()}
	m.Artifacts = nil
	if e = a.Bind(&m, q); e == nil {
		t.Fatal("bound artifacts from a different attempt")
	}
	includeCorrect = true
	if e = a.Bind(&m, q); e != nil {
		t.Fatal(e)
	}
	if len(m.Artifacts) != 2 || m.Artifacts[0].ID != 101 || m.Artifacts[1].ID != 102 {
		t.Fatalf("wrong immutable IDs selected: %#v", m.Artifacts)
	}
}
