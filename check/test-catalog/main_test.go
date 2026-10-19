package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCatalogRejectsInvalidOwnershipAndStages(t *testing.T) {
	for _, tc := range []struct {
		name, image, container string
		valid                  bool
	}{
		{"valid", "check-test-fixture", "FROM scratch AS final\n", true},
		{"wrong owner", "other", "FROM scratch AS final\n", false},
		{"unpinned base", "check-test-fixture", "FROM alpine:latest AS build\nFROM scratch AS final\n", false},
		{"nonfinal scratch", "check-test-fixture", "FROM scratch AS final\nFROM final AS runtime\n", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			dir := filepath.Join(root, "check", "test-fixture")
			if err := os.MkdirAll(dir, 0755); err != nil {
				t.Fatal(err)
			}
			for name, value := range map[string]string{
				"action.yml":    "runs:\n  image: docker://example.invalid/" + tc.image + "@sha256:" + strings.Repeat("a", 64) + "\n",
				"Containerfile": tc.container, "main.go": "package main\n",
			} {
				if err := os.WriteFile(filepath.Join(dir, name), []byte(value), 0644); err != nil {
					t.Fatal(err)
				}
			}
			if err := validate(root); (err == nil) != tc.valid {
				t.Fatalf("validate() = %v, want valid=%t", err, tc.valid)
			}
		})
	}
}
