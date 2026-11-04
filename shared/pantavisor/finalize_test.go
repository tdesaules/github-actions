package pantavisor

import (
	"encoding/json"
	"flag"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestAtomicFinalizationAndAdvancedMain(t *testing.T) {
	// The enclosing test action supplies its own directory input.
	t.Setenv("INPUT_DIRECTORY", "")
	t.Setenv("GIT_CONFIG_GLOBAL", "/dev/null")
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	for _, advanced := range []bool{false, true} {
		t.Run(map[bool]string{false: "atomic", true: "advanced-main"}[advanced], func(t *testing.T) {
			root := t.TempDir()
			source := filepath.Join(root, "source")
			remote := filepath.Join(root, "remote.git")
			os.Mkdir(source, 0755)
			command := func(directory string, args ...string) string {
				t.Helper()
				cmd := exec.Command("git", append([]string{"-C", directory}, args...)...)
				cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=Fixture", "GIT_AUTHOR_EMAIL=fixture@example.invalid", "GIT_COMMITTER_NAME=Fixture", "GIT_COMMITTER_EMAIL=fixture@example.invalid")
				out, err := cmd.CombinedOutput()
				if err != nil {
					t.Fatalf("git %v: %s %v", args, out, err)
				}
				return strings.TrimSpace(string(out))
			}
			command(root, "init", "--bare", "-b", "main", remote)
			command(source, "init", "-b", "main")
			os.WriteFile(filepath.Join(source, "VERSION"), []byte("0.1.0\n"), 0644)
			os.WriteFile(filepath.Join(source, ".gitignore"), []byte("artifacts/\ndist/\n"), 0644)
			command(source, "add", ".")
			command(source, "commit", "-m", "feat: initialize")
			command(source, "push", remote, "main")
			t.Chdir(source)
			t.Setenv("GITHUB_REF", "refs/heads/main")
			t.Setenv("GITHUB_RUN_ID", "123")
			t.Setenv("GITHUB_RUN_ATTEMPT", "1")
			t.Setenv("GITHUB_OUTPUT", "")
			t.Setenv("GITHUB_REPOSITORY", "fixture/pantavisor-core")
			t.Setenv("INPUT_TOKEN", "fixture-not-a-token")
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
			ReadJSON("artifacts/release/candidate.json", &candidate)
			os.Mkdir("dist", 0755)
			os.WriteFile("dist/payload", []byte("qualified"), 0644)
			files, err := Inventory("dist")
			if err != nil {
				t.Fatal(err)
			}
			if err := WriteJSON("artifacts/qualified-receipt.json", Receipt{Schema: 1, Project: "core", Revision: candidate.Revision, Version: candidate.Version, Tested: true, Files: files}); err != nil {
				t.Fatal(err)
			}
			// Git's local fixture rewrite intercepts the exact production HTTPS URL.
			// No GitHub write or real credential is involved in this test.
			command(source, "config", "url.file://"+remote+".insteadOf", "https://github.com/fixture/pantavisor-core.git")
			if advanced {
				writer := filepath.Join(root, "writer")
				command(root, "clone", remote, writer)
				os.WriteFile(filepath.Join(writer, "concurrent"), []byte("change"), 0644)
				command(writer, "add", ".")
				command(writer, "commit", "-m", "fix: concurrent writer")
				command(writer, "push", "origin", "main")
			}
			before := command(root, "--git-dir", remote, "rev-parse", "main")
			flags("--artifact-id=42")
			err = FinalizeCandidate()
			if advanced {
				if err == nil {
					t.Fatal("advanced main accepted")
				}
				if command(root, "--git-dir", remote, "rev-parse", "main") != before || command(root, "--git-dir", remote, "tag", "--list") != "" {
					t.Fatal("partial remote mutation")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if command(root, "--git-dir", remote, "rev-parse", "main") != candidate.Revision || command(root, "--git-dir", remote, "rev-parse", "v0.1.0^{commit}") != candidate.Revision {
				t.Fatal("commit/tag publication mismatch")
			}
			var tagged Candidate
			if err := json.Unmarshal([]byte(command(root, "--git-dir", remote, "for-each-ref", "--format=%(contents:subject)", "refs/tags/v0.1.0")), &tagged); err != nil {
				t.Fatal(err)
			}
			if tagged.ReceiptDigest == "" || tagged.RunID != "123" || tagged.RunAttempt != "1" || tagged.ArtifactID != "42" {
				t.Fatal("tag lost qualification provenance")
			}
			flags("--artifact-id=42")
			if err := FinalizeCandidate(); err == nil {
				t.Fatal("existing release tag accepted for overwrite")
			}
		})
	}
}
