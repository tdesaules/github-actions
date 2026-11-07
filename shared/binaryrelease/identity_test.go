package binaryrelease

import (
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

func TestRunnerIdentitySelection(t *testing.T) {
	for _, tt := range []struct{ output, want string }{{"/runner/output", "/runner/output"}, {"", "/workspace"}} {
		owner, e := runnerIdentity(tt.output, "/workspace", func(path string) (identity, error) {
			if path != tt.want {
				t.Errorf("lookup %s want %s", path, tt.want)
			}
			return identity{1001, 123}, nil
		})
		if e != nil || owner != (identity{1001, 123}) {
			t.Fatalf("owner: %#v %v", owner, e)
		}
	}
	calls := 0
	_, e := runnerIdentity("/missing-output", "/workspace", func(string) (identity, error) { calls++; return identity{}, errors.New("private details") })
	if e == nil || calls != 1 || strings.Contains(e.Error(), "private details") {
		t.Fatal("output lookup did not fail closed with sanitized error")
	}
}

func TestVersionProbeWithoutOutput(t *testing.T) {
	t.Setenv("GITHUB_OUTPUT", "/nonexistent/private-output")
	for _, arg := range []string{"--version", "--tool-version"} {
		if e := RunCLI("fixture 1", []string{arg}, nil, io.Discard, func() error { t.Fatal("probe ran operation"); return nil }); e != nil {
			t.Fatal(e)
		}
	}
}

// Run in an existing action container using an injected static test binary. The
// root parent keeps ownership of the source and checks its unprivileged child.
func TestRunnerIdentityRootChild(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("requires root in an isolated test container")
	}
	if _, e := exec.LookPath("git-cliff"); e != nil {
		t.Skip("requires action git-cliff runtime")
	}
	dir, e := os.MkdirTemp("", "binary-owner-test-")
	if e != nil {
		t.Fatal(e)
	}
	defer os.RemoveAll(dir)
	if e = os.Chmod(dir, 0755); e != nil {
		t.Fatal(e)
	}
	workspace := filepath.Join(dir, "workspace")
	source := filepath.Join(workspace, "source")
	if e = os.MkdirAll(source, 0755); e != nil {
		t.Fatal(e)
	}
	t.Chdir(source)
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	for _, args := range [][]string{{"init", "-b", "main"}, {"config", "user.name", "Fixture"}, {"config", "user.email", "fixture@example.invalid"}, {"config", "commit.gpgsign", "false"}, {"config", "tag.gpgsign", "false"}} {
		if _, e = Git(args...); e != nil {
			t.Fatal(e)
		}
	}
	write(t, "cliff.toml", "[changelog]\nbody = \"{{ version }}\"\n[git]\nconventional_commits = true\n")
	commit(t, "chore: initial")
	if _, e = Git("tag", "1.0.0"); e != nil {
		t.Fatal(e)
	}
	write(t, "source.txt", "original source\n")
	commit(t, "feat: fixture")
	if e = os.Chmod(source, 0555); e != nil {
		t.Fatal(e)
	}
	output := filepath.Join(workspace, "outputs")
	write(t, output, "")
	for _, path := range []string{workspace, output} {
		if e = os.Chown(path, 65532, 65532); e != nil {
			t.Fatal(e)
		}
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestRunnerIdentityChild$", "-test.v")
	cmd.Dir = workspace
	cmd.Env = append(os.Environ(), "BINARY_RUNNER_CHILD=1", "BINARY_RUNNER_SOURCE="+source, "GITHUB_OUTPUT="+output, "HOME=/tmp")
	if b, e := cmd.CombinedOutput(); e != nil {
		t.Fatalf("unprivileged child failed: %v\n%s", e, b)
	}
	info, e := os.Stat(filepath.Join(workspace, ".build", "released", "proof"))
	if e != nil {
		t.Fatal(e)
	}
	owner := info.Sys().(*syscall.Stat_t)
	if owner.Uid != 65532 || owner.Gid != 65532 {
		t.Fatalf("output ownership %d:%d", owner.Uid, owner.Gid)
	}
	sourceInfo, e := os.Stat(source)
	if e != nil {
		t.Fatal(e)
	}
	if sourceInfo.Sys().(*syscall.Stat_t).Uid != 0 || sourceInfo.Mode().Perm() != 0555 {
		t.Fatal("source ownership or read-only mode changed")
	}
}

func TestRunnerIdentityChild(t *testing.T) {
	if os.Getenv("BINARY_RUNNER_CHILD") != "1" {
		t.Skip("subprocess only")
	}
	e := RunCLI("fixture 1", nil, nil, io.Discard, func() error {
		if os.Geteuid() != 65532 || os.Getegid() != 65532 {
			t.Fatalf("identity not adopted: %d:%d", os.Geteuid(), os.Getegid())
		}
		groups, e := os.Getgroups()
		if e != nil || len(groups) != 0 {
			t.Fatalf("supplementary groups retained: %v %v", groups, e)
		}
		if e = os.MkdirAll(".build/released", 0755); e != nil {
			return e
		}
		if e = os.WriteFile(".build/released/proof", []byte("runner-owned\n"), 0644); e != nil {
			return e
		}
		if e = Output("owned", true); e != nil {
			return e
		}
		if e = os.Chdir(os.Getenv("BINARY_RUNNER_SOURCE")); e != nil {
			return e
		}
		if _, e = Git("rev-parse", "HEAD"); e != nil {
			return e
		}
		b, e := runCliff("--offline", "--no-exec", "--config", "cliff.toml", "--bumped-version", "--bump", "minor")
		if e != nil {
			return e
		}
		if strings.TrimSpace(string(b)) != "1.1.0" {
			t.Fatalf("unexpected cliff result %q", b)
		}
		return nil
	})
	if e != nil {
		t.Fatal(e)
	}
}
