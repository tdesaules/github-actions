package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestWorkerBindsOnlyWorkspaceAndOutput(t *testing.T) {
	raw := `{"id":"` + strings.Repeat("a", 64) + `","image":"sha256:` + strings.Repeat("b", 64) + `","mounts":[
{"Type":"bind","Source":"/runner/project with spaces","Destination":"/github/workspace","RW":true},
{"Type":"bind","Source":"/runner/commands","Destination":"/github/file_commands","RW":true},
{"Type":"bind","Source":"/var/run/docker.sock","Destination":"/var/run/docker.sock","RW":true}]}`
	var c actionContainer
	if err := json.Unmarshal([]byte(raw), &c); err != nil {
		t.Fatal(err)
	}
	workspace, output, err := workerMounts(c, strings.Repeat("a", 12), "/github/file_commands/set_output_test")
	if err != nil || workspace != "/runner/project with spaces" || output != "/runner/commands/set_output_test" {
		t.Fatalf("%q %q %v", workspace, output, err)
	}
	if _, _, err := workerMounts(c, strings.Repeat("c", 12), "/github/file_commands/set_output_test"); err == nil {
		t.Fatal("another container's image accepted")
	}
	if _, _, err := workerMounts(c, strings.Repeat("a", 12), "/var/run/docker.sock"); err == nil {
		t.Fatal("engine socket accepted as output bind")
	}
	if _, _, err := workerMounts(c, strings.Repeat("a", 12), "/github/file_commands/save_state_test"); err == nil {
		t.Fatal("a different command file accepted")
	}
	c.Image = "image:latest"
	if _, _, err := workerMounts(c, strings.Repeat("a", 12), "/github/file_commands/set_output_test"); err == nil {
		t.Fatal("mutable worker image accepted")
	}
}

func testContainer(t *testing.T) actionContainer {
	t.Helper()
	var c actionContainer
	if err := json.Unmarshal([]byte(`{"id":"`+strings.Repeat("a", 64)+`","image":"sha256:`+strings.Repeat("b", 64)+`","mounts":[
{"Type":"bind","Source":"/runner/workspace","Destination":"/github/workspace","RW":true},
{"Type":"bind","Source":"/runner/commands","Destination":"/github/file_commands","RW":true},
{"Type":"bind","Source":"/var/run/docker.sock","Destination":"/var/run/docker.sock","RW":true}]}`), &c); err != nil {
		t.Fatal(err)
	}
	return c
}
func TestCoordinatorHasOnlyOwnedWorkerBindings(t *testing.T) {
	c := testContainer(t)
	t.Setenv("GITHUB_TOKEN", "do-not-forward")
	var calls [][]string
	err := coordinate(context.Background(), c, c.ID[:12], "/github/file_commands/set_output_test", "42", "2", []string{"--archive=source.oci.tar", "--output=installer.iso"}, func(ctx context.Context, args []string, capture bool) ([]byte, error) {
		calls = append(calls, args)
		if _, ok := ctx.Deadline(); !ok {
			t.Fatal("unbounded Docker operation")
		}
		if args[0] == "rm" {
			return []byte("Error: No such container"), errors.New("already removed")
		}
		return nil, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(calls) != 2 {
		t.Fatal(calls)
	}
	args := calls[0]
	env := []string{}
	mounts := []string{}
	name := ""
	for i, v := range args {
		switch v {
		case "--env":
			env = append(env, args[i+1])
		case "--mount":
			mounts = append(mounts, args[i+1])
		case "--name":
			name = args[i+1]
		}
	}
	if len(env) != 4 || strings.Join(env, "|") != "ATOMIC_LORAX_WORKER=1|GITHUB_OUTPUT=/github/output|GITHUB_RUN_ID=42|GITHUB_RUN_ATTEMPT=2" {
		t.Fatal(env)
	}
	if len(mounts) != 2 || strings.Contains(strings.Join(args, "|"), "docker.sock") || strings.Contains(strings.Join(args, "|"), "do-not-forward") {
		t.Fatal("socket or credentials forwarded")
	}
	if strings.Join(mounts, "|") != "type=bind,src=/runner/workspace,dst=/github/workspace|type=bind,src=/runner/commands/set_output_test,dst=/github/output" {
		t.Fatal(mounts)
	}
	if !strings.Contains(strings.Join(args, "|"), "|--pull=never|") || args[len(args)-3] != c.Image {
		t.Fatal("worker image identity changed")
	}
	if len(name) != len("atomic-lorax-")+32 || strings.Join(calls[1], "|") != "rm|--force|"+name {
		t.Fatal("cleanup not scoped")
	}
}
func TestCoordinatorCancellationRetainsCleanupFailure(t *testing.T) {
	c := testContainer(t)
	ctx, cancel := context.WithCancel(context.Background())
	err := coordinate(ctx, c, c.ID[:12], "/github/file_commands/set_output_test", "42", "1", nil, func(ctx context.Context, args []string, _ bool) ([]byte, error) {
		if args[0] == "run" {
			cancel()
			return nil, context.Canceled
		}
		if ctx.Err() != nil {
			t.Fatal("cleanup inherited cancellation")
		}
		return nil, errors.New("cleanup fixture failure")
	})
	if err == nil || !strings.Contains(err.Error(), "cleanup fixture failure") || !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}
func TestCoordinatorRejectsAmbiguousMountsAndRunIdentity(t *testing.T) {
	for _, change := range []func(*actionContainer){
		func(c *actionContainer) { c.Mounts = append(c.Mounts, c.Mounts[0]) },
		func(c *actionContainer) { c.Mounts[0].RW = false },
		func(c *actionContainer) { c.Mounts[0].Source = "/" },
		func(c *actionContainer) { c.Mounts[0].Destination = "/github/workspace/nested" },
	} {
		c := testContainer(t)
		change(&c)
		if _, _, err := workerMounts(c, c.ID[:12], "/github/file_commands/set_output_test"); err == nil {
			t.Fatal("unsafe mounts accepted")
		}
	}
	for _, pair := range [][2]string{{"", "1"}, {"42", ""}, {"0", "1"}, {"42", "0"}, {"42\nSECRET=x", "1"}, {"42", "-1"}} {
		if runIdentity(pair[0], pair[1]) == nil {
			t.Fatal("invalid run identity accepted")
		}
	}
	if runIdentity("42", "2") != nil {
		t.Fatal("valid run identity rejected")
	}
	c := testContainer(t)
	if _, _, err := workerMounts(c, c.ID[:12], "/github/file_commands/set_output_x\nkey=value"); err == nil {
		t.Fatal("output command injection accepted")
	}
}
func TestWorkerCommandCancellationStopsChildGroup(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "child")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nsleep 60 &\nwait\n"), 0700); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	start := time.Now()
	cmd := process(ctx, script)
	_, err := cmd.Output()
	if err == nil || time.Since(start) > 4*time.Second {
		t.Fatal("process group did not terminate promptly")
	}
	if _, ok := err.(*exec.ExitError); !ok {
		t.Fatal(err)
	}
}
