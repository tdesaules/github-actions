package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestYAMLStreamsAndDuplicateKeys(t *testing.T) {
	for _, fixture := range []struct {
		data  string
		valid bool
	}{
		{"on: push\n---\nvalue: [1, 2]\n", true},
		{"value: first\nvalue: second\n", false},
		{"value: [unterminated", false},
		{"name: valid\n---\nname: a\nname: b\n", false},
	} {
		path := filepath.Join(t.TempDir(), "file.yaml")
		if err := os.WriteFile(path, []byte(fixture.data), 0600); err != nil {
			t.Fatal(err)
		}
		if err := validate(path); (err == nil) != fixture.valid {
			t.Fatalf("fixture %q: %v", fixture.data, err)
		}
	}
}
