package main

import (
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// Self-reexec avoids shell utilities absent from the published scratch Go
// runtime. The child signals readiness and keeps the parent's capture pipe open,
// so killing only the parent cannot make the cancellation test pass.
func init() {
	for i, arg := range os.Args {
		if !strings.HasPrefix(arg, "--atomic-cancel-helper=") {
			continue
		}
		if i+1 >= len(os.Args) {
			os.Exit(70)
		}
		ready := os.Args[i+1]
		switch strings.TrimPrefix(arg, "--atomic-cancel-helper=") {
		case "parent":
			self, err := os.Executable()
			if err != nil {
				os.Exit(71)
			}
			child := exec.Command(self, "--atomic-cancel-helper=child", ready)
			child.Stdout, child.Stderr = os.Stdout, os.Stderr
			// No Setpgid: inherit the process group created by the code under test.
			if child.Run() != nil {
				os.Exit(72)
			}
			os.Exit(0)
		case "child":
			if os.WriteFile(ready, []byte(strconv.Itoa(os.Getpid())), 0600) != nil {
				os.Exit(73)
			}
			time.Sleep(time.Minute)
			os.Exit(0)
		default:
			os.Exit(74)
		}
	}
}

func waitForCancellationChild(t *testing.T, path string, done <-chan error) {
	t.Helper()
	deadline := time.NewTimer(10 * time.Second)
	defer deadline.Stop()
	tick := time.NewTicker(10 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case err := <-done:
			t.Fatalf("helper exited before its child was ready: %v", err)
		case <-deadline.C:
			t.Fatal("helper child did not become ready")
		case <-tick.C:
			data, err := os.ReadFile(path)
			if os.IsNotExist(err) {
				continue
			}
			if err != nil {
				t.Fatal(err)
			}
			pid, err := strconv.Atoi(string(data))
			// The regular-file write may be observed between creation and write.
			if len(data) == 0 {
				continue
			}
			if err != nil || pid <= 0 {
				t.Fatal("invalid helper child identity")
			}
			t.Cleanup(func() { _ = syscall.Kill(pid, syscall.SIGKILL) })
			return
		}
	}
}
