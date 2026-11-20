package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestApplicationSubjectSelection(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("GIT_CONFIG_GLOBAL", "/dev/null")
	t.Setenv("GITHUB_OUTPUT", "result")
	git := func(args ...string) {
		t.Helper()
		if b, e := exec.Command("git", args...).CombinedOutput(); e != nil {
			t.Fatalf("git: %v: %s", e, b)
		}
	}
	git("init", "-b", "main")
	git("config", "user.name", "fixture")
	git("config", "user.email", "fixture@example.invalid")
	digest := "sha256:" + strings.Repeat("a", 64)
	refs := map[string]string{}
	for _, n := range []string{"bastion", "provision"} {
		if e := os.MkdirAll("oci/"+n, 0755); e != nil {
			t.Fatal(e)
		}
		if e := os.WriteFile("oci/"+n+"/Containerfile", nil, 0644); e != nil {
			t.Fatal(e)
		}
		refs[n] = "ghcr.io/fixture/repo/" + n + "@" + digest
	}
	data, _ := json.Marshal(map[string]any{"images": refs})
	os.WriteFile("definition.json", data, 0644)
	git("add", "oci", "definition.json")
	git("commit", "-m", "initial")
	for _, tc := range []struct{ event, path, want string }{{"maintenance", "", `"provision":"` + digest + `"`}, {"push", "oci/provision/Containerfile", `build_images=["provision"]`}, {"push", ".github/scripts/smoke-bastion.py", `reused={"bastion":"` + digest + `"}`}, {"push", ".grype.yaml", `scan_only=["bastion","provision"]`}, {"push", "docs/guide.md", "has_tests=false"}} {
		os.WriteFile("result", nil, 0600)
		if e := applicationPlan("definition.json", "fixture/repo", tc.event, "refs/heads/main", []string{tc.path}, false); e != nil {
			t.Fatal(e)
		}
		b, _ := os.ReadFile("result")
		if !strings.Contains(string(b), tc.want) {
			t.Fatal(string(b))
		}
	}
	if applicationPlan("definition.json", "fixture/repo", "workflow_dispatch", "refs/heads/topic", nil, true) == nil {
		t.Fatal("accepted manual preparation outside main")
	}
	git("tag", "-a", "1.0.0", "-m", `{"bastion":"`+digest+`","provision":"`+digest+`"}`)
	os.WriteFile("result", nil, 0600)
	if e := applicationPlan("definition.json", "fixture/repo", "workflow_dispatch", "refs/heads/main", nil, true); e != nil {
		t.Fatal(e)
	}
	b, _ := os.ReadFile("result")
	if !strings.Contains(string(b), "build_images=[]") || !strings.Contains(string(b), "has_tests=true") {
		t.Fatal(string(b))
	}
}
