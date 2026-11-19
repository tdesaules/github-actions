// Package chezmoi launches a fixed operation from the trusted consumer checkout.
package chezmoi

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
)

// Run receives the executable and arguments from the action binary, never from inputs.
func Run(executable string, args ...string) error {
	if len(os.Args) != 1 {
		return fmt.Errorf("unexpected command-line arguments")
	}
	dir := os.Getenv("INPUT_DIRECTORY")
	if dir == "" {
		dir = "."
	}
	resolved, err := filepath.EvalSymlinks(dir)
	if err != nil || !filepath.IsLocal(resolved) || !filepath.IsLocal(dir) {
		return fmt.Errorf("directory must be inside the workspace: %q", dir)
	}
	info, err := os.Stat(resolved)
	if err != nil || !info.IsDir() {
		return fmt.Errorf("missing checkout: %q", dir)
	}
	cmd := exec.Command(executable, args...)
	cmd.Dir = resolved
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	if os.Getuid() == 0 {
		owner := info.Sys().(*syscall.Stat_t)
		cmd.SysProcAttr = &syscall.SysProcAttr{Credential: &syscall.Credential{Uid: owner.Uid, Gid: owner.Gid}}
	}
	return cmd.Run()
}
