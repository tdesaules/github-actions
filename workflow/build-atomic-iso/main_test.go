package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestMakeGetsConstructionDirectoryPWD(t *testing.T) {
	if _, err := exec.LookPath("make"); err != nil {
		t.Skip("make not installed")
	}
	directory := t.TempDir()
	t.Setenv("PWD", "/stale-caller-directory")
	if err := os.WriteFile(filepath.Join(directory, "Makefile"), []byte("all:\n\t@test '$(PWD)' = '$(CURDIR)'\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := runCommand(context.Background(), directory, "make"); err != nil {
		t.Fatal("Make template paths used stale PWD:", err)
	}
}

func TestMakeDoesNotOwnActionOutputs(t *testing.T) {
	directory := t.TempDir()
	script := "#!/bin/sh\n[ -z \"${GITHUB_OUTPUT+x}\" ] && [ \"$ATOMIC_TEST_VALUE\" = preserved ]\n"
	if err := os.WriteFile(filepath.Join(directory, "make"), []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", directory+":"+os.Getenv("PATH"))
	t.Setenv("GITHUB_OUTPUT", filepath.Join(directory, "output"))
	t.Setenv("ATOMIC_TEST_VALUE", "preserved")
	if err := runCommand(context.Background(), directory, "make"); err != nil {
		t.Fatal(err)
	}
	if os.Getenv("GITHUB_OUTPUT") == "" {
		t.Fatal("parent action output environment must remain available")
	}
}

func TestSourceIdentity(t *testing.T) {
	hash := strings.Repeat("a", 64)
	if err := validate("ghcr.io/niri-tonic/niri-tomic-os", hash); err != nil {
		t.Fatal(err)
	}
	for _, repo := range []string{"", "ghcr.io/owner/image:latest", "ghcr.io/owner/image;touch", "http://registry/image"} {
		if err := validate(repo, hash); err == nil {
			t.Fatalf("invalid repository accepted: %q", repo)
		}
	}
	if err := validate("ghcr.io/niri-tonic/niri-tomic-os", "unknown"); err == nil {
		t.Fatal("missing checksum accepted")
	}
}

func TestWorkspaceConfinement(t *testing.T) {
	root, outside := t.TempDir(), t.TempDir()
	t.Chdir(root)
	if err := os.WriteFile("source with spaces.oci.tar", []byte("fixture"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := workspacePath("source with spaces.oci.tar", true); err != nil {
		t.Fatal(err)
	}
	if _, err := workspacePath(".build/new/output.iso", false); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"/dev/null", "../outside", outside + "/output.iso"} {
		if _, err := workspacePath(name, false); err == nil {
			t.Fatalf("outside path accepted: %s", name)
		}
	}
	if err := os.Symlink(outside, "escape"); err != nil {
		t.Fatal(err)
	}
	if _, err := workspacePath("escape/output.iso", false); err == nil {
		t.Fatal("symlinked output parent escaped workspace")
	}
	if err := os.Symlink("source with spaces.oci.tar", "linked.oci.tar"); err != nil {
		t.Fatal(err)
	}
	if _, err := workspacePath("linked.oci.tar", true); err == nil {
		t.Fatal("symlinked source archive accepted")
	}
}
