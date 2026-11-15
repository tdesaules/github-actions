package main

import (
	"encoding/json"
	"flag"
	"github.com/tdesaules/github-actions/shared/atomicrelease"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestPlanCandidateAndNoBump(t *testing.T) {
	root := t.TempDir()
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err = os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(old) })
	t.Setenv("GIT_CONFIG_GLOBAL", "/dev/null")
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("GITHUB_SHA", "")
	for _, args := range [][]string{{"init", "-b", "main"}, {"config", "user.name", "Test"}, {"config", "user.email", "test@example.com"}} {
		if out, err := exec.Command("git", args...).CombinedOutput(); err != nil {
			t.Fatalf("%s %v", out, err)
		}
	}
	if err = os.MkdirAll("recipes", 0755); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile("recipes/recipe.yml", []byte("name: fixture\n"), 0644); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"add", "."}, {"commit", "-m", "chore: initialize"}} {
		if out, err := exec.Command("git", args...).CombinedOutput(); err != nil {
			t.Fatalf("%s %v", out, err)
		}
	}
	runPlan := func(initial bool) plan {
		t.Helper()
		oldArgs, oldFlags := os.Args, flag.CommandLine
		defer func() { os.Args = oldArgs; flag.CommandLine = oldFlags }()
		flag.CommandLine = flag.NewFlagSet("test", flag.ContinueOnError)
		os.Args = []string{"plan", "--event=workflow_dispatch", "--ref=refs/heads/main", "--repository=owner/os"}
		if initial {
			os.Args = append(os.Args, "--initial-release=true")
		}
		t.Setenv("GITHUB_OUTPUT", "")
		if err := run(); err != nil {
			t.Fatal(err)
		}
		data, err := os.ReadFile(filepath.Join(".build", "plan.json"))
		if err != nil {
			t.Fatal(err)
		}
		var p plan
		if err = json.Unmarshal(data, &p); err != nil {
			t.Fatal(err)
		}
		return p
	}
	p := runPlan(false)
	if p.HasRelease || p.HasImages || p.HasTests || p.HasLint {
		t.Fatal("no-bump selected work")
	}
	p = runPlan(true)
	if p.Version != "1.0.0" || !p.HasRelease || len(p.Build.Include) != 2 || len(p.Test.Include) != 2 {
		t.Fatal("initial release lacks complete candidate qualification")
	}
	for _, row := range p.Test.Include {
		if row["source"] != "candidate" {
			t.Fatal("build lacks candidate tests")
		}
	}
}

func TestReleaseEvidence(t *testing.T) {
	e := releaseEvidence{
		SchemaVersion: 1, Version: "1.0.0", SourceSHA: strings.Repeat("a", 40),
		Image:   "ghcr.io/niri-tonic/niri-tomic-os@sha256:" + strings.Repeat("b", 64),
		ISO:     "ghcr.io/niri-tonic/niri-tomic-os-iso@sha256:" + strings.Repeat("c", 64),
		ISOFile: "niri-tomic-os-x86_64.iso", ISOSHA256: strings.Repeat("d", 64),
		Architectures: []string{"amd64", "arm64"},
	}
	r := atomicrelease.Receipt{SchemaVersion: 1, SourceSHA: e.SourceSHA, Version: e.Version, Component: "os", Arch: "amd64", Archive: "image.oci.tar", SHA256: strings.Repeat("e", 64), ManifestDigest: "sha256:" + strings.Repeat("f", 64), Tested: true, Qualification: "userspace", RunID: "42", RunAttempt: "1", Checks: map[string]string{"test": "passed"}, Packages: []string{"niri"}}
	r.Checks = map[string]string{
		"Fedora version": "", "Niri configuration": "", "desktop and keyring packages": "",
		"PAM dynamic dependencies": "", "enabled greeter": "", "enabled GPU profile": "",
		"GPU helper syntax": "", "GPU memory ceiling": "",
	}
	e.Qualified.AMD64 = r
	r.Arch = "arm64"
	e.Qualified.ARM64 = r
	r.Arch = "amd64"
	r.Component = "iso"
	r.Archive = e.ISOFile
	r.SHA256 = e.ISOSHA256
	r.Size = 123
	r.OSArchiveSHA256 = e.Qualified.AMD64.SHA256
	r.Qualification = "installer-media"
	r.BuildRunID = "42"
	r.BuildRunAttempt = "1"
	e.Qualified.ISO = r
	e.ISOFiles = []atomicrelease.Payload{{Path: e.ISOFile, SHA256: e.ISOSHA256, MediaType: "application/x-iso9660-image"}, {Path: e.ISOFile + "-CHECKSUM", SHA256: strings.Repeat("a", 64), MediaType: "text/plain"}, {Path: e.ISOFile + ".build.json", SHA256: strings.Repeat("b", 64), MediaType: "application/json"}}
	if err := validateEvidence(e, "1.0.0", "niri-tonic/niri-tomic-os"); err != nil {
		t.Fatal(err)
	}
	if err := validateEvidence(e, "1.0.1", "niri-tonic/niri-tomic-os"); err == nil {
		t.Fatal("wrong release accepted")
	}
	if err := validateEvidence(e, "1.0.0", "other/repository"); err == nil {
		t.Fatal("wrong package identity accepted")
	}
	bad := e
	bad.Image = "ghcr.io/niri-tonic/niri-tomic-os:latest"
	if err := validateEvidence(bad, "1.0.0", "niri-tonic/niri-tomic-os"); err == nil {
		t.Fatal("mutable image reference accepted")
	}
	bad = e
	bad.Architectures = []string{"amd64"}
	if err := validateEvidence(bad, "1.0.0", "niri-tonic/niri-tomic-os"); err == nil {
		t.Fatal("incomplete platform set accepted")
	}
}

func TestRouting(t *testing.T) {
	for _, v := range []struct{ event, ref, want string }{
		{"push", "refs/heads/main", "development"},
		{"workflow_dispatch", "refs/heads/main", "prepare"},
		{"push", "refs/tags/1.0.0", "publish"},
		{"push", "refs/tags/v1.0.0", ""},
		{"push", "refs/tags/1.0.0-rc1", ""},
		{"workflow_dispatch", "refs/heads/topic", ""},
		{"schedule", "refs/heads/main", ""},
	} {
		got, err := mode(v.event, v.ref)
		if got != v.want || (err == nil) != (v.want != "") {
			t.Fatalf("%+v: got %q, %v", v, got, err)
		}
	}
}

func TestMaintenance(t *testing.T) {
	for _, event := range []string{"schedule", "workflow_dispatch"} {
		if err := maintenanceMode(event, "refs/heads/main"); err != nil {
			t.Fatal(err)
		}
		if maintenanceMode(event, "refs/heads/topic") == nil {
			t.Fatal("wrong maintenance ref accepted")
		}
	}
	if maintenanceMode("push", "refs/heads/main") == nil {
		t.Fatal("branch push entered maintenance")
	}
	got := markdownFiles([]string{"z.md", "a.MD", "b.yml", "notes with spaces.md"})
	if strings.Join(got, ",") != "a.MD,notes with spaces.md,z.md" {
		t.Fatal(got)
	}
}

func TestSelection(t *testing.T) {
	lint, build := selection([]string{"README.md"}, false)
	if build || len(lint.Include) != 1 || lint.Include[0]["id"] != "markdownlint" {
		t.Fatal("docs-only must not build")
	}
	_, build = selection([]string{"files/system/usr/libexec/profile"}, false)
	if !build {
		t.Fatal("extensionless helper must trigger build")
	}
	lint, build = selection(nil, false)
	if build || len(lint.Include) != 0 {
		t.Fatal("empty selection must be empty")
	}
	_, build = selection([]string{"recipes/removed.yml"}, false)
	if !build {
		t.Fatal("deleted recipes must trigger build")
	}
}
