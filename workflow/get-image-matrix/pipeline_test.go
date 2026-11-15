package main

import (
	"encoding/json"
	"flag"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func invokePlan(t *testing.T, args ...string) (map[string]string, error) {
	t.Helper()
	oldFlags, oldArgs := flag.CommandLine, os.Args
	flag.CommandLine = flag.NewFlagSet("matrix", flag.ContinueOnError)
	os.Args = append([]string{"matrix"}, args...)
	defer func() { flag.CommandLine, os.Args = oldFlags, oldArgs }()
	output := filepath.Join(t.TempDir(), "outputs")
	if err := os.WriteFile(output, nil, 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GITHUB_OUTPUT", output)
	err := run()
	data, _ := os.ReadFile(output)
	values := map[string]string{}
	for _, line := range strings.Split(string(data), "\n") {
		name, value, ok := strings.Cut(line, "=")
		if ok {
			values[name] = value
		}
	}
	return values, err
}

func TestIndependentImagePlans(t *testing.T) {
	t.Chdir(t.TempDir())
	git := func(args ...string) {
		t.Helper()
		if data, err := exec.Command("git", args...).CombinedOutput(); err != nil {
			t.Fatalf("git: %s %v", data, err)
		}
	}
	git("init", "-b", "main")
	git("config", "user.name", "Fixture")
	git("config", "user.email", "fixture@example.invalid")
	for _, name := range []string{"a", "b"} {
		dir := "lint/" + name
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
		for path, data := range map[string]string{
			dir + "/action.yml":    "runs:\n  using: docker\n  image: docker://ghcr.io/fixture/github-actions/lint-" + name + "@sha256:" + strings.Repeat("a", 64) + "\n",
			dir + "/Containerfile": "FROM scratch AS final\nCOPY " + dir + "/main.go /entrypoint\nCOPY shared/ /shared/\n",
			dir + "/main.go":       "package main\n",
		} {
			if err := os.WriteFile(path, []byte(data), 0644); err != nil {
				t.Fatal(err)
			}
		}
	}
	git("add", ".")
	git("-c", "commit.gpgsign=false", "commit", "-m", "fixture")
	for _, tc := range []struct {
		name, files, event, ref, image string
		images, native                 int
		build, runtime                 bool
	}{
		{"source", `["lint/a/main.go"]`, "push", "refs/heads/main", "", 1, 2, true, true},
		{"readme", `["lint/a/README.md"]`, "push", "refs/heads/main", "", 0, 0, false, false},
		{"metadata without release baseline", `["lint/a/action.yml"]`, "push", "refs/heads/main", "", 1, 2, false, true},
		{"shared", `["shared/deleted.go"]`, "push", "refs/heads/main", "", 2, 4, true, true},
		{"child", `["shared/deleted.go"]`, "push", "refs/heads/main", "lint-b", 1, 2, true, true},
		{"scan only", `["grype-review.json"]`, "push", "refs/heads/main", "", 2, 4, false, false},
		{"tag", `[]`, "push", "refs/tags/1.2.3", "", 2, 4, false, true},
		{"empty", `[]`, "push", "refs/heads/main", "", 0, 0, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			outputs, err := invokePlan(t, "--files="+tc.files, "--event="+tc.event, "--ref="+tc.ref, "--image="+tc.image)
			if err != nil {
				t.Fatal(err)
			}
			var images struct {
				Include []map[string]string `json:"include"`
			}
			var native, builds matrix
			for name, target := range map[string]any{"image-matrix": &images, "matrix": &native, "build-matrix": &builds} {
				if err := json.Unmarshal([]byte(outputs[name]), target); err != nil {
					t.Fatal(err)
				}
			}
			if len(images.Include) != tc.images || len(native.Include) != tc.native || (len(builds.Include) > 0) != tc.build {
				t.Fatalf("unexpected plan: %v", outputs)
			}
			for _, j := range native.Include {
				if j.RuntimeTest != tc.runtime {
					t.Fatalf("incorrect runtime selection: %+v", j)
				}
			}
		})
	}
	for _, args := range [][]string{
		{"--event=maintenance", "--ref=refs/heads/other"},
		{"--event=workflow_dispatch", "--ref=refs/heads/other"},
		{"--event=push", "--ref=refs/tags/1.2.3-rc.1"},
		{"--event=push", "--ref=refs/heads/main", "--image=unknown"},
	} {
		if _, err := invokePlan(t, args...); err == nil {
			t.Fatalf("accepted invalid plan: %v", args)
		}
	}
	if err := os.WriteFile("lint/a/action.yml", []byte("runs:\n  using: docker\n  image: Containerfile\n"), 0644); err != nil {
		t.Fatal(err)
	}
	for _, event := range []string{"maintenance", "schedule"} {
		outputs, err := invokePlan(t, "--event="+event, "--ref=refs/heads/main")
		if err != nil || outputs["mode"] != "maintenance" {
			t.Fatalf("unpublished action blocked maintenance: %v %v", outputs, err)
		}
		if _, exists := outputs["image-matrix"]; exists {
			t.Fatal("maintenance must not plan image builds")
		}
	}
	if _, err := invokePlan(t, "--event=push", "--ref=refs/tags/1.2.3"); err == nil {
		t.Fatal("release accepted an unpublished action")
	}
}
