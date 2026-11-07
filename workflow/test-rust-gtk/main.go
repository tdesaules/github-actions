package main

import (
	"context"
	"fmt"
	"github.com/tdesaules/github-actions/shared/rustgtk"
	"github.com/tdesaules/github-actions/shared/workflow"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

func smoke(binary, home string) error {
	runtimeDir := filepath.Join(home, "runtime")
	if e := os.Mkdir(runtimeDir, 0700); e != nil {
		return e
	}
	env := []string{"HOME=" + home, "XDG_CONFIG_HOME=" + filepath.Join(home, "config"), "XDG_CACHE_HOME=" + filepath.Join(home, "cache"), "XDG_DATA_HOME=" + filepath.Join(home, "data"), "XDG_RUNTIME_DIR=" + runtimeDir, "WAYLAND_DISPLAY=rust-gtk-wayland", "GDK_BACKEND=wayland", "GSK_RENDERER=cairo", "LIBGL_ALWAYS_SOFTWARE=1", "NO_AT_BRIDGE=1"}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	weston := exec.CommandContext(ctx, "weston", "--backend=headless", "--renderer=pixman", "--socket=rust-gtk-wayland", "--idle-time=0", "--no-config", "--log="+filepath.Join(home, "weston.log"))
	weston.Env = append(os.Environ(), env...)
	weston.Stdout = os.Stdout
	weston.Stderr = os.Stderr
	if e := weston.Start(); e != nil {
		return e
	}
	defer func() { _ = weston.Process.Kill(); _ = weston.Wait() }()
	ready := false
	for i := 0; i < 100; i++ {
		if _, e := os.Stat(filepath.Join(runtimeDir, "rust-gtk-wayland")); e == nil {
			ready = true
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if !ready {
		b, _ := os.ReadFile(filepath.Join(home, "weston.log"))
		return fmt.Errorf("Weston readiness timeout: %s", b)
	}
	cmd := exec.CommandContext(ctx, "dbus-run-session", "--", binary, "--smoke-test")
	cmd.Env = append(os.Environ(), env...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
func copyExact(src, dst string) error {
	i, e := os.Open(src)
	if e != nil {
		return e
	}
	defer i.Close()
	o, e := os.Create(dst)
	if e != nil {
		return e
	}
	_, e = io.Copy(o, i)
	ce := o.Close()
	if e != nil {
		return e
	}
	return ce
}
func qualify(v rustgtk.Inputs) error {
	r, e := rustgtk.ReadReceipt(v.Receipt)
	if e != nil {
		return e
	}
	if e = rustgtk.Verify(v, r); e != nil {
		return e
	}
	if e = rustgtk.Source(v.SourceSHA); e != nil {
		return e
	}
	tmp, e := os.MkdirTemp("", "rust-gtk-test-")
	if e != nil {
		return e
	}
	defer os.RemoveAll(tmp)
	extracted := filepath.Join(tmp, "candidate")
	if e = rustgtk.Extract(v.Archive, extracted); e != nil {
		return e
	}
	binary := filepath.Join(extracted, "bin", rustgtk.Component)
	home := filepath.Join(tmp, "home")
	if e = os.Mkdir(home, 0700); e != nil {
		return e
	}
	env := []string{"HOME=" + home, "XDG_CONFIG_HOME=" + filepath.Join(home, "config"), "XDG_CACHE_HOME=" + filepath.Join(home, "cache"), "XDG_DATA_HOME=" + filepath.Join(home, "data"), "PYTHONDONTWRITEBYTECODE=1", "NIRI_TONIC_RELEASE_VERSION=" + v.Version, "CARGO_TARGET_DIR=" + filepath.Join(tmp, "target")}
	for _, c := range []struct {
		dir, name string
		args      []string
	}{{"", "cargo", []string{"test", "--locked", "-j2"}}, {"plugins/filen", "npm", []string{"ci", "--ignore-scripts", "--no-audit", "--no-fund"}}, {"plugins/filen", "npm", []string{"test"}}, {"plugins/google-drive", "npm", []string{"test"}}, {"", "python3", []string{"scripts/check-connectors.py", "--protocol-only"}}} {
		if e = rustgtk.Run(c.dir, env, c.name, c.args...); e != nil {
			return e
		}
	}
	if info, err := os.Stat(".github/scripts"); err == nil {
		if !info.IsDir() {
			return fmt.Errorf(".github/scripts must be a directory")
		}
		if e = rustgtk.Run("", env, "python3", "-m", "unittest", "discover", "-s", ".github/scripts", "-p", "test_*.py"); e != nil {
			return e
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	versionContext, cancelVersion := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancelVersion()
	cmd := exec.CommandContext(versionContext, binary, "--version")
	cmd.Env = append(os.Environ(), env...)
	out, e := cmd.Output()
	if e != nil {
		return e
	}
	if strings.TrimSpace(string(out)) != rustgtk.Component+" "+v.Version {
		return fmt.Errorf("binary version mismatch: %q", out)
	}
	if e = rustgtk.Run("", env, binary, "--check-config"); e != nil {
		return e
	}
	if e = smoke(binary, home); e != nil {
		return e
	}
	if e = rustgtk.Source(v.SourceSHA); e != nil {
		return e
	}
	if e = rustgtk.Verify(v, r); e != nil {
		return e
	}
	if e = os.MkdirAll(v.Output, 0755); e != nil {
		return e
	}
	archive := filepath.Join(v.Output, r.Archive)
	if archive == v.Archive {
		return fmt.Errorf("qualification output must differ from candidate directory")
	}
	if e = copyExact(v.Archive, archive); e != nil {
		return e
	}
	hash, e := rustgtk.Hash(archive)
	if e != nil {
		return e
	}
	if hash != r.SHA256 {
		return fmt.Errorf("qualified copy checksum mismatch")
	}
	r.Tested = true
	receipt := archive + ".receipt.json"
	if e = rustgtk.JSON(receipt, r); e != nil {
		return e
	}
	// Export a fresh extraction of the checked copy, never a source checkout or
	// the temporary payload potentially touched by functional tests.
	payload, e := os.MkdirTemp(v.Output, "payload-")
	if e != nil {
		return e
	}
	if e = os.Chmod(payload, 0755); e != nil {
		return e
	}
	if e = rustgtk.Extract(archive, payload); e != nil {
		return e
	}
	if base := os.Getenv("GITHUB_WORKSPACE"); base != "" {
		if relative, err := filepath.Rel(base, payload); err == nil {
			payload = relative
		}
	}
	if e = workflow.Output("extracted-root", payload); e != nil {
		return e
	}
	return rustgtk.Outputs(archive, receipt, hash)
}
func run() error {
	v, e := rustgtk.Flags("test")
	if e != nil {
		return e
	}
	if e = rustgtk.Paths(&v); e != nil {
		return e
	}
	e = qualify(v)
	report := map[string]any{"schema_version": 1, "passed": e == nil, "source": v.Source, "verification_run_id": os.Getenv("GITHUB_RUN_ID"), "verification_run_attempt": os.Getenv("GITHUB_RUN_ATTEMPT")}
	if e != nil {
		report["error"] = e.Error()
	}
	if re := rustgtk.JSON(filepath.Join(v.Output, "test-report.json"), report); re != nil {
		return re
	}
	return e
}
func main() { rustgtk.Main(run) }
