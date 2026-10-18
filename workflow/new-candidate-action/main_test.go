package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCandidateRetainsInputsAndLeavesSourceUnchanged(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source with spaces")
	target := filepath.Join(root, "candidate")
	if err := os.MkdirAll(source, 0755); err != nil {
		t.Fatal(err)
	}
	metadata := "name: fixture\ninputs:\n  files:\n    description: files\nruns:\n  using: docker\n  image: docker://example.invalid/tool@sha256:abc\n  args:\n    - --files=${{ inputs.files }}\n"
	if err := os.WriteFile(filepath.Join(source, "action.yml"), []byte(metadata), 0644); err != nil {
		t.Fatal(err)
	}
	if err := execute([]string{"--directory=" + source, "--destination=" + target, "--image=localhost/fixture:tested"}); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(filepath.Join(target, "action.yml"))
	if !strings.Contains(string(data), "--files=${{ inputs.files }}") || !strings.Contains(string(data), "image: Dockerfile") {
		t.Fatal(string(data))
	}
	original, _ := os.ReadFile(filepath.Join(source, "action.yml"))
	if string(original) != metadata {
		t.Fatal("source changed")
	}
	for _, image := range []string{"localhost/image:tag\nRUN injected", "example.invalid/image:tag"} {
		if err := execute([]string{"--directory=" + source, "--destination=" + target, "--image=" + image}); err == nil {
			t.Fatal("accepted invalid reference")
		}
	}
}

func TestRegistryCandidateIsPulledByExactDigest(t *testing.T) {
	root := t.TempDir()
	source, target := filepath.Join(root, "source"), filepath.Join(root, "candidate")
	if err := os.Mkdir(source, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "action.yml"), []byte("runs:\n  using: docker\n  image: docker://original/image:unpublished\n"), 0644); err != nil {
		t.Fatal(err)
	}
	image := "localhost:5000/candidate@sha256:" + strings.Repeat("a", 64)
	if err := execute([]string{"--directory=" + source, "--destination=" + target, "--image=" + image}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(target, "action.yml"))
	if err != nil || !strings.Contains(string(data), "docker://"+image) {
		t.Fatal(string(data), err)
	}
	if _, err := os.Stat(filepath.Join(target, "Dockerfile")); !os.IsNotExist(err) {
		t.Fatal("registry candidate unexpectedly requires a build")
	}
}
