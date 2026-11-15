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

// The published scratch Go runtime has no sleep executable. Self-reexec creates
// an actual child holding the captured output pipe, without shell dependencies.
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
			if len(data) == 0 {
				continue
			}
			pid, err := strconv.Atoi(string(data))
			if err != nil || pid <= 0 {
				t.Fatal("invalid helper child identity")
			}
			t.Cleanup(func() { _ = syscall.Kill(pid, syscall.SIGKILL) })
			return
		}
	}
}
