package main

import (
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"
)

func runFromEnvironment(t *testing.T) error {
	t.Helper()
	oldFlags, oldArgs := flag.CommandLine, os.Args
	flag.CommandLine = flag.NewFlagSet("test-step-outcomes", flag.ContinueOnError)
	os.Args = []string{"test-step-outcomes"}
	t.Cleanup(func() { flag.CommandLine, os.Args = oldFlags, oldArgs })
	return run()
}

func TestActionForwardsJSONThroughEnvironment(t *testing.T) {
	data, err := os.ReadFile("action.yml")
	if err != nil {
		t.Fatal(err)
	}
	var action struct {
		Runs struct {
			Env  map[string]string `yaml:"env"`
			Args []string          `yaml:"args"`
		} `yaml:"runs"`
	}
	if err := yaml.Unmarshal(data, &action); err != nil {
		t.Fatal(err)
	}
	for env, input := range map[string]string{
		"INPUT_OUTCOMES": "outcomes", "INPUT_EXPECTED-FAILURES": "expected-failures", "INPUT_ABSENT-FILES": "absent-files",
	} {
		if action.Runs.Env[env] != "${{ inputs."+input+" }}" {
			t.Errorf("%s is not explicitly forwarded", env)
		}
		for _, arg := range action.Runs.Args {
			if strings.Contains(arg, input) {
				t.Fatalf("JSON argv override would bypass environment transport: %s", arg)
			}
		}
	}
}

func TestNestedFixtureJSONKeepsAssertionsAndSecurityOutcomes(t *testing.T) {
	dir := t.TempDir()
	// Reproduce the hosted toJSON(steps) shape: JSON serialized inside an output
	// string, plus paths containing quotes/backslashes and a continued failure.
	nested := `[{"path":"quoted \"payload\" with spaces.txt","sha256":"` + strings.Repeat("a", 64) + `","media_type":"text/plain"}]`
	outcomes, err := json.Marshal(map[string]any{
		"atomic-protocol": map[string]any{"outcome": "success", "outputs": map[string]string{
			"iso-files": nested, "windows-path": `C:\fixtures\payload.json`, "multiline": "line one\nline two",
		}},
		"negative":      map[string]string{"outcome": "failure", "conclusion": "success"},
		"security-scan": map[string]string{"outcome": "failure", "conclusion": "success"},
		"security-sbom": map[string]string{"outcome": "success", "conclusion": "success"},
	})
	if err != nil {
		t.Fatal(err)
	}
	absent := filepath.Join(dir, `absent "quoted" \ path.json`)
	paths, _ := json.Marshal([]string{absent})
	t.Setenv("INPUT_OUTCOMES", string(outcomes))
	t.Setenv("INPUT_EXPECTED-FAILURES", `["negative"]`)
	t.Setenv("INPUT_ABSENT-FILES", string(paths))
	t.Setenv("SECURITY_SUBJECT", "sha256:"+strings.Repeat("b", 64))
	t.Setenv("SECURITY_OUTPUT", filepath.Join(dir, "security.json"))
	t.Setenv("SECURITY_REPORT", filepath.Join(dir, "missing-grype.json"))
	t.Setenv("GITHUB_STEP_SUMMARY", "")
	output := filepath.Join(dir, "outputs")
	if err := os.WriteFile(output, nil, 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GITHUB_OUTPUT", output)
	if err := runFromEnvironment(t); err != nil {
		t.Fatal(err)
	}
	var reports []securityResult
	data, err := os.ReadFile(filepath.Join(dir, "security.json"))
	if err != nil || json.Unmarshal(data, &reports) != nil || len(reports) != 2 {
		t.Fatalf("security evidence lost: %s, %v", data, err)
	}
	foundFailure := false
	for _, r := range reports {
		if r.Operation == "security-scan" {
			foundFailure = r.Outcome == "failure" && r.Status == "error"
		}
	}
	if !foundFailure {
		t.Fatal("continued scanner failure was lost or relabeled")
	}
	// The same environment must still enforce required negative results.
	t.Setenv("INPUT_EXPECTED-FAILURES", `["atomic-protocol"]`)
	if err := runFromEnvironment(t); err == nil || !strings.Contains(err.Error(), "expected failure") {
		t.Fatalf("successful step accepted as a required failure: %v", err)
	}
	t.Setenv("INPUT_EXPECTED-FAILURES", `["negative"]`)
	if err := os.WriteFile(absent, []byte("invalid leftover"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := runFromEnvironment(t); err == nil || !strings.Contains(err.Error(), "left an output") {
		t.Fatalf("quoted leftover output escaped validation: %v", err)
	}
}
