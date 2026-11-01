package runtimeaction

import (
	"os"
	"path/filepath"
	"testing"
)

func TestIdentityTokenFilePermissionsAndCleanup(t *testing.T) {
	path, cleanup, err := IdentityTokenFile("private-test-token")
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	for p, want := range map[string]os.FileMode{path: 0600, filepath.Dir(path): 0700} {
		info, err := os.Stat(p)
		if err != nil || info.Mode().Perm() != want {
			t.Fatalf("unexpected permissions for %s: %v", p, err)
		}
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != "private-test-token" {
		t.Fatal("token transfer failed")
	}
	cleanup()
	if _, err := os.Stat(filepath.Dir(path)); !os.IsNotExist(err) {
		t.Fatal("token directory retained")
	}
}
