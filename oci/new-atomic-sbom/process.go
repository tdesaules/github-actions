package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"
)

type command struct {
	Name, Work string
	Args       []string
	Capture    bool
}
type commandResult struct {
	Output  []byte
	PeakRSS int64
}
type runner func(context.Context, command) (commandResult, error)

type limitedBuffer struct{ bytes.Buffer }

func (b *limitedBuffer) Write(p []byte) (int, error) {
	if b.Len()+len(p) > 16<<20 {
		return 0, errors.New("tool metadata exceeds 16 MiB")
	}
	return b.Buffer.Write(p)
}

// No ambient Syft settings, credential helpers, proxy credentials, user config,
// package exclusions or target-OS executables are inherited by these tools.
func toolEnvironment(work string) []string {
	return []string{"PATH=/usr/bin:/bin", "HOME=" + filepath.Join(work, "home"),
		"XDG_CONFIG_HOME=" + filepath.Join(work, "config"), "TMPDIR=" + work,
		"GOMEMLIMIT=4GiB", "SYFT_PARALLELISM=1", "SYFT_CHECK_FOR_APP_UPDATE=false",
		"SSL_CERT_FILE=/etc/ssl/certs/ca-certificates.crt"}
}
func execute(ctx context.Context, c command) (commandResult, error) {
	if c.Name != "skopeo" && c.Name != "umoci" && c.Name != "syft" && c.Name != "rpm" {
		return commandResult{}, errors.New("unsupported internal tool")
	}
	cmd := exec.CommandContext(ctx, "/usr/bin/"+c.Name, c.Args...)
	cmd.Dir = c.Work
	cmd.Env = toolEnvironment(c.Work)
	cmd.Stderr = os.Stderr
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
	cmd.WaitDelay = 5 * time.Second
	var out limitedBuffer
	if c.Capture {
		cmd.Stdout = &out
	} else {
		cmd.Stdout = os.Stderr
	}
	err := cmd.Run()
	result := commandResult{Output: out.Bytes()}
	if cmd.ProcessState != nil {
		if usage, ok := cmd.ProcessState.SysUsage().(*syscall.Rusage); ok {
			result.PeakRSS = usage.Maxrss * 1024
		}
	}
	if err != nil {
		// Never serialize stdin, authentication config, command arguments or the
		// captured response into errors/source-binding evidence.
		if ctx.Err() != nil {
			return result, fmt.Errorf("%s cancelled: %w", c.Name, ctx.Err())
		}
		return result, fmt.Errorf("%s failed: %w; see tool diagnostics", c.Name, err)
	}
	return result, nil
}
