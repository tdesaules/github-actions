package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSourceMetadataRequiresExplicitQualification(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "workflow", "fixture")
	if err := os.MkdirAll(directory, 0755); err != nil {
		t.Fatal(err)
	}
	for name, data := range map[string]string{
		"action.yml":    "name: Fixture\ndescription: Fixture\ninputs:\n  source-sha:\n    description: Source commit\noutputs:\n  source_sha:\n    description: Source commit\nruns:\n  using: docker\n  image: Containerfile\n  env:\n    INPUT_SOURCE_SHA: ${{ inputs.source-sha }}\n",
		"main.go":       "package main\n",
		"Containerfile": "FROM scratch AS final\nLABEL org.opencontainers.image.title=\"workflow-fixture\"\n",
	} {
		if err := os.WriteFile(filepath.Join(directory, name), []byte(data), 0644); err != nil {
			t.Fatal(err)
		}
	}
	path := filepath.Join(directory, "action.yml")
	t.Setenv("ALLOW_SOURCE_ACTIONS", "")
	if err := validate(path); err == nil {
		t.Fatal("source metadata accepted without qualification opt-in")
	}
	t.Setenv("ALLOW_SOURCE_ACTIONS", "true")
	t.Setenv("GITHUB_REF", "refs/heads/fixture")
	if err := validate(path); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GITHUB_REF", "refs/tags/1.0.0")
	if err := validate(path); err == nil {
		t.Fatal("source metadata accepted on release tag")
	}
}
