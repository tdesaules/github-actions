package main

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDownloadIntegrity(t *testing.T) {
	directory := t.TempDir()
	data := []byte("immutable payload")
	files := []payload{{Path: "installer with spaces.iso", SHA256: fmt.Sprintf("%x", sha256.Sum256(data))}}
	if err := os.WriteFile(filepath.Join(directory, files[0].Path), data, 0444); err != nil {
		t.Fatal(err)
	}
	if err := verify(directory, files); err != nil {
		t.Fatal(err)
	}
	if err := validate(append(files, files[0])); err == nil {
		t.Fatal("duplicate expected file accepted")
	}
	bad := []payload{{Path: files[0].Path, SHA256: strings.Repeat("0", 64)}}
	if err := verify(directory, bad); err == nil {
		t.Fatal("tampered bytes accepted")
	}
	if err := os.WriteFile(filepath.Join(directory, "unexpected"), data, 0444); err != nil {
		t.Fatal(err)
	}
	if err := verify(directory, files); err == nil {
		t.Fatal("unexpected payload accepted")
	}
}

func TestRejectUnsafePayloads(t *testing.T) {
	for _, path := range []string{"", ".", "..", "../file", "/file", "-option", "file:media", "file\nname"} {
		if err := validate([]payload{{Path: path, SHA256: strings.Repeat("a", 64)}}); err == nil {
			t.Fatalf("unsafe name accepted: %q", path)
		}
	}
	directory := t.TempDir()
	if err := os.Symlink("/etc/passwd", filepath.Join(directory, "payload")); err != nil {
		t.Fatal(err)
	}
	if err := verify(directory, []payload{{Path: "payload", SHA256: strings.Repeat("a", 64)}}); err == nil {
		t.Fatal("symlink payload accepted")
	}
}
