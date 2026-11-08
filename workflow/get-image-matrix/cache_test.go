package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

func TestReuseBuildRequiresSuccessfulJobAndEquivalentSources(t *testing.T) {
	for _, scenario := range []string{"reuse", "expired", "missing", "failed job", "wrong artifact source", "rerun race", "wrong repository", "api outage", "duplicate"} {
		t.Run(scenario, func(t *testing.T) {
			git, write := sourceFixture(t)
			git("add", ".")
			git("commit", "-m", "source")
			base := git("rev-parse", "HEAD")
			write("oci/a/main.go", "changed\n")
			git("add", ".")
			git("commit", "-m", "next source")
			head := git("rev-parse", "HEAD")
			run := cacheRun{ID: 12, SHA: base, Branch: "main", Path: ".github/workflows/main.yml", Event: "push", Status: "completed", Attempt: 1}
			run.HeadRepository.FullName = "owner/repo"
			if scenario == "wrong repository" {
				run.HeadRepository.FullName = "other/repo"
			}
			artifact := cacheArtifact{ID: 123, Name: "build-oci-b-amd64-12"}
			artifact.WorkflowRun.ID = 12
			artifact.WorkflowRun.SHA = base
			if scenario == "expired" {
				artifact.Expired = true
			}
			if scenario == "wrong artifact source" {
				artifact.WorkflowRun.SHA = head
			}
			pages := map[string]int{}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Authorization") != "Bearer fixture" {
					t.Error("missing authentication")
				}
				pages[r.URL.Path+"?"+r.URL.RawQuery]++
				switch {
				case strings.HasSuffix(r.URL.Path, "/actions/workflows/main.yml/runs"):
					json.NewEncoder(w).Encode(map[string]any{"workflow_runs": []cacheRun{run}})
				case strings.HasSuffix(r.URL.Path, "/artifacts"):
					if scenario == "api outage" {
						w.WriteHeader(503)
						return
					}
					// Realistic catalogue size: the wanted artifact is on page two.
					items := []cacheArtifact{}
					if r.URL.Query().Get("page") == "1" {
						for i := 0; i < 100; i++ {
							items = append(items, cacheArtifact{Name: fmt.Sprintf("unrelated-%d", i)})
						}
					} else if scenario != "missing" {
						items = append(items, artifact)
						if scenario == "duplicate" {
							items = append(items, artifact)
						}
					}
					json.NewEncoder(w).Encode(map[string]any{"artifacts": items})
				case strings.HasSuffix(r.URL.Path, "/attempts/1/jobs"):
					items := []cacheJob{}
					if r.URL.Query().Get("page") == "1" {
						for i := 0; i < 100; i++ {
							items = append(items, cacheJob{Name: fmt.Sprintf("other-%d", i)})
						}
					} else {
						conclusion := "success"
						if scenario == "failed job" {
							conclusion = "failure"
						}
						items = append(items, cacheJob{Name: "build (oci-b, amd64)", Conclusion: conclusion, SHA: base})
					}
					json.NewEncoder(w).Encode(map[string]any{"jobs": items})
				case strings.HasSuffix(r.URL.Path, "/actions/runs/12"):
					current := run
					if scenario == "rerun race" {
						current.Attempt = 2
					}
					json.NewEncoder(w).Encode(current)
				default:
					t.Errorf("unexpected endpoint %s", r.URL)
					w.WriteHeader(404)
				}
			}))
			defer server.Close()
			plan := matrix{Include: []job{
				{Image: "oci-a", Directory: "oci/a", Arch: "amd64", Runner: "ubuntu-24.04", Rebuild: true, Scan: true},
				{Image: "oci-b", Directory: "oci/b", Arch: "amd64", Runner: "ubuntu-24.04", Rebuild: true, Scan: true},
				{Image: "oci-b", Directory: "oci/b", Arch: "arm64", Runner: "ubuntu-24.04-arm", Rebuild: true, Scan: true},
			}}
			before := append([]job(nil), plan.Include...)
			cache := buildCache{client: server.Client(), endpoint: server.URL, repository: "owner/repo", token: "fixture"}
			err := cache.reuse(&plan, head, "main")
			if scenario == "api outage" || scenario == "rerun race" {
				if err == nil || !reflect.DeepEqual(before, plan.Include) {
					t.Fatalf("failure must retain full build plan: %v %+v", err, plan)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			want := ""
			if scenario == "reuse" {
				want = "123"
			}
			if plan.Include[0].ArtifactID != "" || plan.Include[2].ArtifactID != "" || plan.Include[1].ArtifactID != want {
				t.Fatalf("incorrect reuse: %+v", plan)
			}
			if want != "" {
				j := plan.Include[1]
				if !j.Rebuild || !j.Scan || j.SourceRevision != base || j.SourceRun != "12" || j.SourceVersion != "0.0.0-sha-"+base[:12]+"-12" {
					t.Fatalf("lost qualification or provenance: %+v", j)
				}
				for _, j := range buildMatrix(plan).Include {
					if j.Image == "oci-b" && j.Arch == "amd64" {
						t.Fatal("cached product still scheduled for build")
					}
				}
			}
		})
	}
}

func TestCacheIdentityIncludesCopiedDocumentationAndDeletedInputs(t *testing.T) {
	for _, name := range []string{"README.md", "asset"} {
		t.Run(name, func(t *testing.T) {
			git, write := sourceFixture(t)
			write("oci/a/README.md", "old")
			git("add", ".")
			git("commit", "-m", "base")
			base := git("rev-parse", "HEAD")
			if name == "asset" {
				git("rm", "oci/a/asset")
			} else {
				write("oci/a/README.md", "new")
				git("add", ".")
			}
			git("commit", "-m", "change")
			ok, err := equivalentSources("oci/a", base, git("rev-parse", "HEAD"))
			if err != nil || ok {
				t.Fatalf("copied input change reused: %v %v", ok, err)
			}
		})
	}
}
