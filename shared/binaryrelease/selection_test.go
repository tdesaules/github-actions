package binaryrelease

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"
)

func lintRows(m Matrix) map[string][]string {
	rows := map[string][]string{}
	for _, r := range m.Include {
		rows[r["id"].(string)] = r["files"].([]string)
	}
	return rows
}

func TestSpecializedLintSelection(t *testing.T) {
	fixtureRepo(t)
	for path, data := range map[string]string{"docs/space name.md": "# Guide\n", "README.md": "# App\n", ".github/workflows/main.yml": "name: main\n", "config/settings.yaml": "enabled: true\n", "cliff.toml": "[bump]\ninitial_tag = \"1.0.0\"\n", ".markdownlint.json": "{}\n"} {
		write(t, path, data)
	}
	source := commit(t, "chore: fixtures")
	cases := []struct {
		name    string
		changed []string
		want    map[string][]string
	}{
		{"docs", []string{"docs/space name.md"}, map[string][]string{"markdownlint": {"docs/space name.md"}}},
		{"markdown config", []string{".markdownlint.json"}, map[string][]string{"markdownlint": {"README.md", "docs/space name.md"}}},
		{"workflow", []string{".github/workflows/main.yml"}, map[string][]string{"actionlint": {".github/workflows/main.yml"}, "yaml": {".github/workflows/main.yml"}}},
		{"cliff", []string{"cliff.toml"}, map[string][]string{"toml": {"cliff.toml"}}},
		{"deleted docs", []string{"docs/deleted.md"}, map[string][]string{"markdownlint": {"README.md", "docs/space name.md"}}},
		{"empty", nil, map[string][]string{}},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			got, e := selectLints(source, tt.changed, false)
			if e != nil {
				t.Fatal(e)
			}
			if !reflect.DeepEqual(lintRows(got), tt.want) {
				t.Fatalf("got %#v want %#v", lintRows(got), tt.want)
			}
			for _, row := range got.Include {
				if row["action_path"] != "lint/"+row["id"].(string) || row["runner"] != "ubuntu-24.04" {
					t.Fatalf("bad row: %#v", row)
				}
			}
		})
	}
	full, e := selectLints(source, nil, true)
	if e != nil {
		t.Fatal(e)
	}
	if len(full.Include) != 5 {
		t.Fatalf("full selection: %#v", full)
	}
	if e = os.Remove(".github/workflows/main.yml"); e != nil {
		t.Fatal(e)
	}
	source = commit(t, "chore: delete workflow")
	deleted, e := selectLints(source, []string{".github/workflows/main.yml"}, false)
	if e != nil {
		t.Fatal(e)
	}
	rows := lintRows(deleted)
	if _, ok := rows["actionlint"]; ok {
		t.Fatal("actionlint selected without workflows")
	}
	if !reflect.DeepEqual(rows["yaml"], []string{"config/settings.yaml"}) {
		t.Fatalf("deleted workflow validation: %#v", rows)
	}
}

func TestPlanningConservativeRuntimeAndHistory(t *testing.T) {
	source := fixtureRepo(t)
	for _, path := range []string{"cliff.toml", ".github/workflows/main.yml", ".github/scripts/test_workflow.py", "scripts/check-connectors.py", "config.example.yaml", "LICENSE", "runtime/adapter.py", "plugins/adapter.py"} {
		before := source
		write(t, path, "fixture\n")
		source = commit(t, "chore: update "+path)
		p, e := MakePlan(PlanOptions{Event: "push", Ref: "refs/heads/main", SourceSHA: source, Before: before, Config: "cliff.toml"})
		if e != nil {
			t.Fatal(e)
		}
		if !p.HasImages || !p.HasTests {
			t.Fatalf("missing runtime qualification for %s", path)
		}
		for _, r := range p.BuildMatrix.Include {
			if r["directory"] != "." {
				t.Fatalf("missing build directory: %#v", r)
			}
		}
		for _, r := range p.TestMatrix.Include {
			if r["source"] != "candidate" || r["runtime_test"] != true || r["scan"] != true {
				t.Fatalf("invalid test row: %#v", r)
			}
			if _, exists := r["digest"]; exists {
				t.Fatal("binary matrix invents a digest")
			}
		}
	}
	for _, before := range []string{"", "not-a-sha", strings.Repeat("b", 40)} {
		if _, e := MakePlan(PlanOptions{Event: "push", Ref: "refs/heads/main", SourceSHA: source, Before: before}); e == nil {
			t.Fatalf("accepted unavailable/invalid base %q", before)
		}
	}
}

func TestCaseInsensitiveBumps(t *testing.T) {
	for message, want := range map[string]string{"FEAT(ui): add": "minor", "FiX: repair": "patch", "PERF: improve": "patch", "CHORE!: remove": "major", "docs: migration\n\nBreaking Change: removed": "major", "docs: migration\n\nbreaking-change: removed": "major"} {
		if got := Bump([]string{message}); got != want {
			t.Fatalf("%q: got %s want %s", message, got, want)
		}
	}
}

func TestJSONConfigurationRequiresSyntaxLintWithoutBinaryRebuild(t *testing.T) {
	before := fixtureRepo(t)
	write(t, "renovate.json", "{invalid JSON\n")
	source := commit(t, "ci: update dependency configuration")
	p, err := MakePlan(PlanOptions{Event: "push", Ref: "refs/heads/main", SourceSHA: source, Before: before})
	if err != nil {
		t.Fatal(err)
	}
	if !p.HasLint || p.HasImages || p.HasTests {
		t.Fatalf("unexpected selection: %#v", p)
	}
	if len(p.LintMatrix.Include) != 1 || p.LintMatrix.Include[0]["id"] != "rust-gtk" {
		t.Fatalf("JSON configuration was not selected for syntax validation: %#v", p.LintMatrix)
	}
}

func TestActionInputEnvironmentMappings(t *testing.T) {
	for _, name := range []string{"plan-rust-gtk-release", "prepare-binary-release", "read-binary-release", "publish-binary-release"} {
		var metadata struct {
			Inputs map[string]any `yaml:"inputs"`
			Runs   struct {
				Env map[string]string `yaml:"env"`
			} `yaml:"runs"`
		}
		b, e := os.ReadFile(filepath.Join("..", "..", "workflow", name, "action.yml"))
		if e != nil {
			t.Fatal(e)
		}
		if e = yaml.Unmarshal(b, &metadata); e != nil {
			t.Fatal(e)
		}
		if len(metadata.Inputs) != len(metadata.Runs.Env) {
			t.Fatalf("%s: input/env cardinality mismatch", name)
		}
		for input := range metadata.Inputs {
			key := "INPUT_" + strings.ToUpper(strings.ReplaceAll(input, "-", "_"))
			want := "${{ inputs." + input + " }}"
			if metadata.Runs.Env[key] != want {
				t.Errorf("%s: %s must map %s", name, key, want)
			}
		}
	}
}

func TestQualificationIgnoresReportsButRejectsExtraArchives(t *testing.T) {
	dir, m := qualifiedFixture(t)
	write(t, filepath.Join(dir, "report.json"), `{"status":"passed"}`)
	write(t, filepath.Join(dir, "amd64", "test-report.json"), `{"tests":12}`)
	if _, e := Verify(dir, m.SourceSHA, m.Version, m.RunID, m.RunAttempt); e != nil {
		t.Fatalf("report JSON parsed as receipt: %v", e)
	}
	write(t, filepath.Join(dir, "unbound.tar.gz"), "unexpected bytes")
	if _, e := Verify(dir, m.SourceSHA, m.Version, m.RunID, m.RunAttempt); e == nil {
		t.Fatal("accepted unbound archive")
	}
}
