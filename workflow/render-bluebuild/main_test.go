package main

import "testing"

func TestInputs(t *testing.T) {
	for _, test := range []struct {
		name, recipe, output, repository, platform string
		valid                                      bool
	}{
		{"amd64", "recipes/recipe.yml", ".build/Containerfile", "ghcr.io/niri-tonic/niri-tomic-os", "linux/amd64", true},
		{"arm64 spaces", "recipes/my recipe.yml", ".build/my Containerfile", "ghcr.io/org/image", "linux/arm64", true},
		{"escape", "../recipe.yml", "Containerfile", "ghcr.io/org/image", "linux/amd64", false},
		{"option instead of recipe", "--help", "Containerfile", "ghcr.io/org/image", "linux/amd64", false},
		{"absolute output", "recipe.yml", "/etc/passwd", "ghcr.io/org/image", "linux/amd64", false},
		{"overwrite recipe", "recipe.yml", "./recipe.yml", "ghcr.io/org/image", "linux/amd64", false},
		{"tag", "recipe.yml", "Containerfile", "ghcr.io/org/image:latest", "linux/amd64", false},
		{"missing namespace", "recipe.yml", "Containerfile", "ghcr.io", "linux/amd64", false},
		{"unknown arch", "recipe.yml", "Containerfile", "ghcr.io/org/image", "linux/386", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := validate(test.recipe, test.output, test.repository, test.platform)
			if (err == nil) != test.valid {
				t.Fatalf("valid=%v, error=%v", test.valid, err)
			}
		})
	}
}
