package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSourceBootstrapIsExplicitAndOwned(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "lint", "rust-gtk")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	for name, text := range map[string]string{
		"action.yml":    "runs:\n  using: docker\n  image: Containerfile\n",
		"main.go":       "package main\n",
		"Containerfile": "FROM scratch AS final\nLABEL org.opencontainers.image.title=\"lint-rust-gtk\"\n",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(text), 0644); err != nil {
			t.Fatal(err)
		}
	}
	if err := validate(root); err == nil {
		t.Fatal("released catalog accepted source metadata")
	}
	if err := validateSource(root, true); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "Containerfile"), []byte("FROM scratch AS final\nLABEL org.opencontainers.image.title=\"other\"\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := validateSource(root, true); err == nil {
		t.Fatal("source bootstrap accepted another owner")
	}
}
