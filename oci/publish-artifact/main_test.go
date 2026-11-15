package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPayloadIntegrity(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "example iso.iso")
	if err := os.WriteFile(path, []byte("qualified bytes"), 0444); err != nil {
		t.Fatal(err)
	}
	hash, err := checksum(path)
	if err != nil {
		t.Fatal(err)
	}
	file := payload{Path: filepath.Base(path), SHA256: hash, MediaType: "application/x-iso9660-image"}
	if err := validatePayloads(dir, []payload{file}); err != nil {
		t.Fatal(err)
	}
	if err := validatePayloads(dir, []payload{file, file}); err == nil {
		t.Fatal("duplicate payload accepted")
	}
	for _, name := range []string{"", ".", "..", "../escape", "/absolute", "-option", "name:media", "line\nbreak"} {
		bad := file
		bad.Path = name
		if err := validatePayloads(dir, []payload{bad}); err == nil {
			t.Fatalf("invalid name accepted: %q", name)
		}
	}
	bad := file
	bad.SHA256 = strings.Repeat("0", 64)
	if err := validatePayloads(dir, []payload{bad}); err == nil {
		t.Fatal("tampered payload accepted")
	}
	if err := os.Symlink(path, filepath.Join(dir, "linked.iso")); err != nil {
		t.Fatal(err)
	}
	bad = file
	bad.Path = "linked.iso"
	if err := validatePayloads(dir, []payload{bad}); err == nil {
		t.Fatal("symlink accepted")
	}
	if err := validatePayloads(dir, nil); err == nil {
		t.Fatal("empty publication accepted")
	}
}
