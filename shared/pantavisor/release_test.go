package pantavisor

import (
	"flag"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestReleaseVersions(t *testing.T) {
	for _, test := range []struct {
		current, previous, messages, want string
		changed                           bool
	}{
		{"0.1.0", "", "feat: initialize", "0.1.0", true},
		{"0.1.0", "v0.1.0", "docs: explain runtime", "0.1.0", false},
		{"0.1.0", "v0.1.0", "fix(core): preserve state", "0.1.1", true},
		{"0.1.0", "v0.1.0", "feat: support a board", "0.2.0", true},
		{"0.1.0", "v0.1.0", "feat!: new runtime ABI", "1.0.0", true},
		{"0.1.0", "v0.1.0", "fix: migrate\n\nBREAKING CHANGE: paths", "1.0.0", true},
	} {
		got, changed, err := nextVersion(test.current, test.previous, test.messages)
		if err != nil || got != test.want || changed != test.changed {
			t.Fatalf("%+v: %s %v %v", test, got, changed, err)
		}
	}
	if _, _, err := nextVersion("invalid", "", "feat: init"); err == nil {
		t.Fatal("invalid VERSION accepted")
	}
}
func TestCandidateBundleRetainsExactCommit(t *testing.T) {
	// The enclosing test action supplies its own directory input.
	t.Setenv("INPUT_DIRECTORY", "")
	t.Setenv("GIT_CONFIG_GLOBAL", "/dev/null")
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	root := t.TempDir()
	source := filepath.Join(root, "source")
	os.Mkdir(source, 0755)
	runGit := func(dir string, args ...string) string {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=Fixture", "GIT_AUTHOR_EMAIL=fixture@example.invalid", "GIT_COMMITTER_NAME=Fixture", "GIT_COMMITTER_EMAIL=fixture@example.invalid")
		data, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %s %v", args, data, err)
		}
		return strings.TrimSpace(string(data))
	}
	runGit(source, "init", "-b", "main")
	os.WriteFile(filepath.Join(source, "VERSION"), []byte("0.1.0\n"), 0644)
	os.WriteFile(filepath.Join(source, ".gitignore"), []byte("artifacts/\ndist/\n"), 0644)
	runGit(source, "add", ".")
	runGit(source, "commit", "-m", "feat: initialize")
	base := runGit(source, "rev-parse", "HEAD")
	t.Chdir(source)
	t.Setenv("GITHUB_REF", "refs/heads/main")
	t.Setenv("GITHUB_RUN_ID", "123")
	t.Setenv("GITHUB_OUTPUT", "")
	t.Setenv("GIT_CONFIG_COUNT", "0")
	originalFlags, originalArgs := flag.CommandLine, os.Args
	t.Cleanup(func() { flag.CommandLine = originalFlags; os.Args = originalArgs })
	flags := func(args ...string) {
		flag.CommandLine = flag.NewFlagSet("fixture", flag.ContinueOnError)
		os.Args = append([]string{"fixture"}, args...)
	}
	flags("--project=core")
	if err := NewCandidate(); err != nil {
		t.Fatal(err)
	}
	var candidate Candidate
	if err := ReadJSON("artifacts/release/candidate.json", &candidate); err != nil {
		t.Fatal(err)
	}
	if candidate.Base != base || candidate.Revision == base || runGit(source, "rev-parse", "main") != base {
		t.Fatal("preparation moved main or lost base identity")
	}
	target := filepath.Join(root, "consumer")
	runGit(root, "clone", "--no-checkout", source, target)
	runGit(target, "checkout", "--detach", base)
	os.MkdirAll(filepath.Join(target, "artifacts/release"), 0755)
	for _, file := range []string{"candidate.json", "candidate.bundle"} {
		data, err := os.ReadFile(filepath.Join(source, "artifacts/release", file))
		if err != nil {
			t.Fatal(err)
		}
		os.WriteFile(filepath.Join(target, "artifacts/release", file), data, 0644)
	}
	if err := os.Chdir(target); err != nil {
		t.Fatal(err)
	}
	flags()
	if err := ImportCandidate(); err != nil {
		t.Fatal(err)
	}
	if runGit(target, "rev-parse", "HEAD") != candidate.Revision {
		t.Fatal("candidate reconstructed instead of imported")
	}
	flags()
	if err := ImportCandidate(); err == nil {
		t.Fatal("import accepted a checkout other than its base")
	}
}
