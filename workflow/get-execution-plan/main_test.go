package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestGitDiffOutputsAndCheckoutMismatch(t *testing.T) {
	root := t.TempDir()
	command := func(args ...string) string {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", root}, args...)...)
		data, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git: %v %s", err, data)
		}
		return strings.TrimSpace(string(data))
	}
	command("init", "-b", "main")
	command("config", "user.name", "Fixture")
	command("config", "user.email", "fixture@example.invalid")
	command("config", "commit.gpgsign", "false")
	path := filepath.Join(root, "README with spaces.md")
	if err := os.WriteFile(path, []byte("# Initial\n"), 0644); err != nil {
		t.Fatal(err)
	}
	command("add", ".")
	command("commit", "-m", "docs: initial")
	base := command("rev-parse", "HEAD")
	if err := os.WriteFile(path, []byte("# Updated\n"), 0644); err != nil {
		t.Fatal(err)
	}
	command("add", ".")
	command("commit", "-m", "docs: update")
	head := command("rev-parse", "HEAD")
	output := filepath.Join(t.TempDir(), "output")
	if err := os.WriteFile(output, nil, 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GITHUB_OUTPUT", output)
	if err := execute([]string{"--repository=" + root, "--before=" + base, "--after=" + head}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	values := map[string]string{}
	for _, line := range strings.Split(string(data), "\n") {
		key, value, ok := strings.Cut(line, "=")
		if ok {
			values[key] = value
		}
	}
	if values["has-work"] != "false" || values["has-markdown"] != "true" {
		t.Fatal(values)
	}
	var files []string
	if err := json.Unmarshal([]byte(values["markdown-files"]), &files); err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 || files[0] != "README with spaces.md" {
		t.Fatal(files)
	}
	if err := execute([]string{"--repository=" + root, "--before=" + base, "--after=" + base}); err == nil {
		t.Fatal("accepted checkout mismatch")
	}
}
