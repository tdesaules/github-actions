package binaryrelease

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCLIInputPrecedenceAndEmpty(t *testing.T) {
	t.Setenv("INPUT_SOURCE_SHA", "from-env")
	t.Setenv("INPUT_MODE", "prepare")
	for _, tt := range []struct {
		args []string
		want string
	}{
		{nil, "from-env"}, {[]string{"--source-sha=from-cli"}, "from-cli"}, {[]string{"--source-sha", "separate-value"}, "separate-value"}, {[]string{"--source-sha="}, ""},
	} {
		e := RunCLI("fixture 1", tt.args, []string{"source-sha", "mode"}, io.Discard, func() error {
			if got := Input("source-sha", "fallback"); got != tt.want {
				t.Errorf("got %q want %q", got, tt.want)
			}
			if got := Input("mode", "fallback"); got != "prepare" {
				t.Errorf("unoverridden env lost: %q", got)
			}
			if got := Input("absent", "fallback"); got != "fallback" {
				t.Errorf("default lost: %q", got)
			}
			return nil
		})
		if e != nil {
			t.Fatal(e)
		}
		if Input("source-sha", "fallback") != "from-env" {
			t.Fatal("CLI overrides leaked after invocation")
		}
	}
}

func TestCLIRejectsInvalidArgumentsWithoutEcho(t *testing.T) {
	for _, args := range [][]string{{"--unknown=private-value"}, {"--token=private-value"}, {"private-value"}, {"--source-sha"}, {"--version", "--source-sha=private-value"}, {"--", "private-value"}} {
		ran := false
		var output bytes.Buffer
		e := RunCLI("fixture 1", args, []string{"source-sha"}, &output, func() error { ran = true; return nil })
		if e == nil || e.Error() != "invalid command-line arguments" || ran || output.Len() != 0 {
			t.Fatalf("invalid arguments not rejected cleanly: %v, ran=%t", e, ran)
		}
	}
	for _, arg := range []string{"--version", "--tool-version"} {
		var output bytes.Buffer
		e := RunCLI("fixture 1", []string{arg}, nil, &output, func() error { t.Fatal("version probe ran operation"); return nil })
		if e != nil || output.String() != "fixture 1\n" {
			t.Fatalf("version probe: %q %v", output.String(), e)
		}
	}
}

func TestPlannerDirectoryKeepsWorkspaceOutput(t *testing.T) {
	source := fixtureRepo(t)
	repo, e := os.Getwd()
	if e != nil {
		t.Fatal(e)
	}
	workspace := t.TempDir()
	relative := filepath.Join(".build", "fixtures", "rust app with spaces")
	fixture := filepath.Join(workspace, relative)
	if e = os.MkdirAll(filepath.Dir(fixture), 0755); e != nil {
		t.Fatal(e)
	}
	if _, e = Git("clone", "--no-hardlinks", repo, fixture); e != nil {
		t.Fatal(e)
	}
	t.Chdir(workspace)
	write(t, "outputs", "")
	t.Setenv("GITHUB_OUTPUT", "outputs")
	t.Setenv("INPUT_EVENT", "push")
	t.Setenv("INPUT_REF", "refs/heads/main")
	t.Setenv("INPUT_SOURCE_SHA", source)
	t.Setenv("INPUT_BEFORE", source)
	t.Setenv("INPUT_OUTPUT", "wrong-env.json")
	args := []string{"--directory", relative, "--output", "results/plan.json"}
	if e = RunCLI("planner 1", args, []string{"directory", "output"}, io.Discard, PlanAction); e != nil {
		t.Fatal(e)
	}
	var p Plan
	if e = Decode("results/plan.json", &p); e != nil {
		t.Fatal(e)
	}
	if p.SourceSHA != source || p.HasLint || p.HasImages || p.HasTests {
		t.Fatalf("unexpected empty fixture plan: %#v", p)
	}
	if _, e = os.Stat(filepath.Join(fixture, "results", "plan.json")); !os.IsNotExist(e) {
		t.Fatal("output was resolved relative to application directory")
	}
	if _, e = os.Stat("wrong-env.json"); !os.IsNotExist(e) {
		t.Fatal("environment output overrode CLI")
	}
	current, e := os.Getwd()
	if e != nil || current != workspace {
		t.Fatalf("workspace not restored: %q %v", current, e)
	}
	output, e := os.ReadFile("outputs")
	if e != nil || !strings.Contains(string(output), "source_sha="+source) {
		t.Fatalf("relative GitHub output not written in original workspace: %s %v", output, e)
	}
}
