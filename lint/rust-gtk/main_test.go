package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLockMismatch(t *testing.T) {
	d := t.TempDir()
	os.WriteFile(filepath.Join(d, "package.json"), []byte(`{"name":"plugin","version":"1.0.0","dependencies":{"sdk":"1"}}`), 0644)
	os.WriteFile(filepath.Join(d, "package-lock.json"), []byte(`{"name":"plugin","version":"1.0.0","packages":{"":{"dependencies":{"sdk":"2"}}}}`), 0644)
	if lockCoherent(d) == nil {
		t.Fatal("accepted stale dependency lock")
	}
}
