package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestFixtureArchivesAreDeterministicAndArchitectureSpecific(t *testing.T) {
	root := t.TempDir()
	old := failureBinary
	oldSuccess := successBinary
	failureBinary = filepath.Join(root, "fail-run")
	successBinary = failureBinary
	t.Cleanup(func() { failureBinary = old; successBinary = oldSuccess })
	if err := os.WriteFile(failureBinary, []byte("fixture"), 0755); err != nil {
		t.Fatal(err)
	}
	first, second, arm := filepath.Join(root, "first.tar"), filepath.Join(root, "second.tar"), filepath.Join(root, "arm.tar")
	for path, arch := range map[string]string{first: "amd64", second: "amd64", arm: "arm64"} {
		if err := archive(path, arch); err != nil {
			t.Fatal(err)
		}
	}
	a, _ := os.ReadFile(first)
	b, _ := os.ReadFile(second)
	c, _ := os.ReadFile(arm)
	if !bytes.Equal(a, b) {
		t.Fatal("identical fixtures changed archive checksum")
	}
	if bytes.Equal(a, c) {
		t.Fatal("architecture ignored")
	}
	if err := execute([]string{"--directory=" + filepath.Join(root, "fixture with spaces")}); err != nil {
		t.Fatal(err)
	}
}
