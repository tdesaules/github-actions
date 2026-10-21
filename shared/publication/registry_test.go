package publication

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRegistryMissingIsNotAnyFailure(t *testing.T) {
	for _, tc := range []struct {
		name, script    string
		missing, failed bool
	}{
		{"present", "printf '{}'", false, false},
		{"missing", "echo 'manifest unknown' >&2; exit 1", true, false},
		{"unauthorized", "echo 'unauthorized' >&2; exit 1", false, true},
		{"network", "echo 'TLS handshake timeout' >&2; exit 1", false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, "skopeo"), []byte("#!/bin/sh\n"+tc.script+"\n"), 0755); err != nil {
				t.Fatal(err)
			}
			t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
			data, err := rawOptional("example.invalid/catalog@sha256:fixture")
			if (err != nil) != tc.failed {
				t.Fatal(err)
			}
			if err == nil && (data == nil) != tc.missing {
				t.Fatalf("unexpected registry result %q", data)
			}
		})
	}
}
