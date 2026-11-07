package binaryrelease

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// git-cliff uses libgit2, which does not inherit git's per-command -c options.
// Scope ownership trust to this checkout in a private, temporary Git home. This
// also works for a read-only root-owned fixture after dropping to the runner UID.
func runCliff(args ...string) ([]byte, error) {
	cwd, e := os.Getwd()
	if e != nil {
		return nil, e
	}
	home, e := os.MkdirTemp("", "binary-cliff-home-")
	if e != nil {
		return nil, e
	}
	defer os.RemoveAll(home)
	config := filepath.Join(home, ".gitconfig")
	if _, e = Git("config", "--file", config, "--add", "safe.directory", cwd); e != nil {
		return nil, fmt.Errorf("cannot configure scoped git-cliff trust")
	}
	cmd := exec.Command("git-cliff", args...)
	cmd.Env = append(os.Environ(), "HOME="+home, "XDG_CONFIG_HOME="+home, "GIT_CONFIG_GLOBAL="+config)
	return cmd.Output()
}
