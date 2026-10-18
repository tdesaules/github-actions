package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestTOMLSyntaxAndDuplicateKeys(t *testing.T) {
	for _, fixture := range []struct {
		data  string
		valid bool
	}{
		{"[package]\nname = 'hello world'\n", true},
		{"name = 'first'\nname = 'second'\n", false},
		{"name = [unterminated", false},
	} {
		path := filepath.Join(t.TempDir(), "file.toml")
		if err := os.WriteFile(path, []byte(fixture.data), 0600); err != nil {
			t.Fatal(err)
		}
		if err := validate(path); (err == nil) != fixture.valid {
			t.Fatalf("fixture %q: %v", fixture.data, err)
		}
	}
}
