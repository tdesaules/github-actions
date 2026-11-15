package atomicrelease

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestBumpPolicy(t *testing.T) {
	for _, tc := range []struct{ messages, want string }{
		{"docs: explain\n", ""}, {"chore: deps\x00fix: bug", "patch"}, {"perf: faster", "patch"},
		{"feat(ui): feature\x00fix: bug", "minor"}, {"chore!: break", "major"},
		{"FEAT: feature", "minor"}, {"PERF: faster", "patch"},
		{"docs: migration\n\nBREAKING CHANGE: incompatible config", "major"},
		{"feat: feature\x00fix: bug\n\nBREAKING-CHANGE: incompatible", "major"},
	} {
		if got := Bump(tc.messages); got != tc.want {
			t.Fatalf("%q: %q, want %q", tc.messages, got, tc.want)
		}
	}
}
func initRepo(t *testing.T) string {
	t.Helper()
	t.Setenv("GIT_CONFIG_GLOBAL", "/dev/null")
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	dir := t.TempDir()
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err = os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(old) })
	runGit(t, "init", "-b", "main")
	runGit(t, "config", "user.name", "Test")
	runGit(t, "config", "user.email", "test@example.com")
	if err = os.WriteFile("source", []byte("source"), 0644); err != nil {
		t.Fatal(err)
	}
	runGit(t, "add", ".")
	runGit(t, "commit", "-m", "chore: initialize")
	return runGit(t, "rev-parse", "HEAD")
}
func runGit(t *testing.T, args ...string) string {
	t.Helper()
	s, e := git(args...)
	if e != nil {
		t.Fatal(args, e)
	}
	return s
}
func TestInitialAndNoBump(t *testing.T) {
	initRepo(t)
	v, changed, err := ReleaseVersion(false)
	if err != nil || changed || v != "" {
		t.Fatalf("%q %v %v", v, changed, err)
	}
	v, changed, err = ReleaseVersion(true)
	if err != nil || !changed || v != "1.0.0" {
		t.Fatalf("%q %v %v", v, changed, err)
	}
	runGit(t, "tag", "-a", "1.0.0", "-m", "release")
	runGit(t, "commit", "--allow-empty", "-m", "docs: explain")
	v, changed, err = ReleaseVersion(false)
	if err != nil || changed || v != "1.0.0" {
		t.Fatalf("%q %v %v", v, changed, err)
	}
	if _, _, err = ReleaseVersion(true); err == nil {
		t.Fatal("repeated initial accepted")
	}
}
func TestRealCliffBump(t *testing.T) {
	if _, err := exec.LookPath("git-cliff"); err != nil {
		t.Skip("git-cliff not installed")
	}
	for _, tc := range []struct{ base, message, want string }{
		{"1.0.0", "perf: faster", "1.0.1"}, {"1.0.0", "fix: bug", "1.0.1"}, {"1.0.0", "FEAT: desktop", "1.1.0"},
		{"1.2.3", "chore!: incompatible", "2.0.0"}, {"0.2.3", "docs: migration\n\nBREAKING CHANGE: config", "0.3.0"},
	} {
		t.Run(tc.message, func(t *testing.T) {
			initRepo(t)
			runGit(t, "tag", "-a", tc.base, "-m", "release")
			runGit(t, "commit", "--allow-empty", "-m", tc.message)
			v, changed, err := ReleaseVersion(false)
			if err != nil || !changed || v != tc.want {
				t.Fatalf("%q %v %v", v, changed, err)
			}
		})
	}
}
func TestMetadataRejectsInterveningSourceAndLightweightTags(t *testing.T) {
	source := initRepo(t)
	e := evidenceFixture(t)
	e.SourceSHA = source
	e.Qualified.AMD64.SourceSHA = source
	e.Qualified.ARM64.SourceSHA = source
	e.Qualified.ISO.SourceSHA = source
	if err := Write(".github/atomic-release.json", e); err != nil {
		t.Fatal(err)
	}
	notes := "# Changelog\n\n## [Unreleased]\n\n## [1.2.3] - 2026-11-15\n\n### Added\n\n- Desktop.\n\n[1.2.3]: https://github.com/owner/os/compare/1.2.2...1.2.3\n"
	if err := os.WriteFile("CHANGELOG.md", []byte(notes), 0644); err != nil {
		t.Fatal(err)
	}
	runGit(t, "add", ".")
	runGit(t, "commit", "-m", "chore(release): prepare 1.2.3")
	runGit(t, "tag", "1.2.3")
	if _, _, err := ReadTag("1.2.3", "owner/os"); err == nil {
		t.Fatal("lightweight tag accepted")
	}
	runGit(t, "tag", "-d", "1.2.3")
	runGit(t, "tag", "-a", "1.2.3", "-m", "release")
	if _, _, err := ReadTag("1.2.3", "owner/os"); err != nil {
		t.Fatal(err)
	}
	runGit(t, "tag", "-d", "1.2.3")
	if err := os.WriteFile(filepath.Join("source"), []byte("changed source"), 0644); err != nil {
		t.Fatal(err)
	}
	runGit(t, "add", ".")
	runGit(t, "commit", "--amend", "--no-edit")
	runGit(t, "tag", "-a", "1.2.3", "-m", "release")
	if _, _, err := ReadTag("1.2.3", "owner/os"); err == nil {
		t.Fatal("source change accepted")
	}
}
func TestRaceAndRollbackGuards(t *testing.T) {
	source := strings.Repeat("a", 40)
	final := strings.Repeat("b", 40)
	if err := CheckRemote(source+"\trefs/heads/main", source); err != nil {
		t.Fatal(err)
	}
	for _, refs := range []string{final + "\trefs/heads/main", source + "\trefs/heads/main\n" + final + "\trefs/tags/1.0.0", ""} {
		if CheckRemote(refs, source) == nil {
			t.Fatal("main/tag race accepted")
		}
	}
	refs := source + "\trefs/tags/1.0.0\n" + final + "\trefs/tags/1.0.0^{}"
	if latest, err := TagStatus(refs, "1.0.0", final); err != nil || !latest {
		t.Fatal(err)
	}
	if latest, err := TagStatus(refs+"\n"+source+"\trefs/tags/1.0.1", "1.0.0", final); err != nil || latest {
		t.Fatal("older stable release must publish without channel promotion")
	}
	if _, err := TagStatus(refs, "1.0.0", source); err == nil {
		t.Fatal("moved remote tag accepted")
	}
}
func TestWrongRefBeforeSideEffects(t *testing.T) {
	t.Setenv("GITHUB_EVENT_NAME", "workflow_dispatch")
	t.Setenv("GITHUB_REF", "refs/heads/topic")
	if PrepareAction(Options{}) == nil {
		t.Fatal("non-main preparation accepted")
	}
	t.Setenv("GITHUB_EVENT_NAME", "push")
	t.Setenv("GITHUB_REF", "refs/tags/v1.0.0")
	if PublishAction(Options{Version: "1.0.0"}) == nil {
		t.Fatal("wrong tag accepted")
	}
}

func TestAtomicPushRejectsTagCollisionWithoutAdvancingMain(t *testing.T) {
	source := initRepo(t)
	remote := filepath.Join(t.TempDir(), "origin.git")
	runGit(t, "init", "--bare", remote)
	runGit(t, "push", remote, "HEAD:refs/heads/main")
	runGit(t, "tag", "-a", "1.0.0", "-m", "old tag")
	runGit(t, "push", remote, "refs/tags/1.0.0")
	runGit(t, "tag", "-d", "1.0.0")
	runGit(t, "commit", "--allow-empty", "-m", "chore(release): metadata")
	final := runGit(t, "rev-parse", "HEAD")
	runGit(t, "tag", "-a", "1.0.0", "-m", "new tag")
	_, err := authenticatedGit("fixture-unused", "push", "--atomic", "--force-with-lease=refs/heads/main:"+source, remote, final+":refs/heads/main", "refs/tags/1.0.0:refs/tags/1.0.0")
	if err == nil {
		t.Fatal("tag collision was accepted")
	}
	if got := runGit(t, "--git-dir="+remote, "rev-parse", "refs/heads/main"); got != source {
		t.Fatal("failed atomic push advanced main")
	}
	_, err = authenticatedGit("fixture-unused", "push", "--atomic", "--force-with-lease=refs/heads/main:"+strings.Repeat("f", 40), remote, final+":refs/heads/main", "refs/tags/1.0.0:refs/tags/1.0.1")
	if err == nil {
		t.Fatal("stale main lease was accepted")
	}
	if _, err := git("--git-dir="+remote, "rev-parse", "--verify", "refs/tags/1.0.1"); err == nil {
		t.Fatal("failed atomic push created tag")
	}
	_, err = authenticatedGit("fixture-unused", "push", "--atomic", "--force-with-lease=refs/heads/main:"+source, remote, final+":refs/heads/main", "refs/tags/1.0.0:refs/tags/1.0.1")
	if err != nil {
		t.Fatal(err)
	}
	if got := runGit(t, "--git-dir="+remote, "rev-parse", "refs/heads/main"); got != final {
		t.Fatal("atomic success did not advance main")
	}
}
