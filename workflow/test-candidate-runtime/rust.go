package main

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/tdesaules/github-actions/shared/candidate"
	"github.com/tdesaules/github-actions/shared/catalog"
)

func rustCases(root *candidate.Root, image, source string, results *[]result) error {
	env := []string{"GITHUB_RUN_ID=123", "GITHUB_RUN_ATTEMPT=1", "GITHUB_OUTPUT=/reports/rust-outputs", "CARGO_TARGET_DIR=/tmp/rust-target", "NIRI_TONIC_RELEASE_VERSION=1.0.0"}
	if err := os.WriteFile("/reports/rust-outputs", nil, 0666); err != nil {
		return err
	}
	if err := os.Chmod("/reports/rust-outputs", 0666); err != nil {
		return err
	}
	run := func(name string, args []string, success bool, diagnostic string) error {
		return actionCase(root, results, name, "/fixtures/rust-app with spaces", args, env, success, diagnostic)
	}
	base := []string{"--directory=/fixtures/rust-app with spaces"}
	if image == "lint-rust-gtk" {
		if err := run("read-only Rust lint source", append(base, "--output=/reports/lint.json"), true, ""); err != nil {
			return err
		}
		var report struct {
			Passed bool `json:"passed"`
		}
		if err := catalog.Read("/reports/lint.json", &report); err != nil {
			return err
		}
		if !report.Passed {
			return fmt.Errorf("lint did not report success")
		}
		return run("missing Rust lint source", []string{"--directory=/fixtures/missing", "--output=/reports/rejected-lint.json"}, false, "")
	}
	base = append(base, "--version=1.0.0", "--source-sha="+source, "--arch="+runtime.GOARCH)
	archive := "/reports/candidate/niri-tonic-files-1.0.0-linux-" + runtime.GOARCH + ".tar.gz"
	if image == "workflow-build-rust-gtk" {
		if err := run("native Rust release packaging", append(base, "--output=/reports/candidate"), true, ""); err != nil {
			return err
		}
		var receipt struct {
			Tested    bool
			SHA256    string
			SourceSHA string `json:"source_sha"`
		}
		if err := catalog.Read(archive+".receipt.json", &receipt); err != nil {
			return err
		}
		data, err := os.ReadFile(archive)
		if err != nil {
			return err
		}
		if receipt.Tested || receipt.SHA256 != fmt.Sprintf("%x", sha256.Sum256(data)) || receipt.SourceSHA != source {
			return fmt.Errorf("invalid untested binary build receipt")
		}
		return run("reject mismatched binary source", append(base, "--source-sha="+strings.Repeat("0", 40), "--output=/reports/rejected-build"), false, "source HEAD mismatch")
	}
	// Construct an actual native Rust executable with the candidate's own toolchain,
	// then test that exact archive. This setup does not invoke the build operation.
	root.Config.Config.Env = append(root.Config.Config.Env, "NIRI_TONIC_RELEASE_VERSION=1.0.0", "CARGO_TARGET_DIR=/tmp/rust-test-fixture")
	out, code, err := root.Run("/usr/bin/cargo", "build", "--manifest-path=/fixtures/rust-app with spaces/Cargo.toml", "--release", "--locked", "--offline")
	if err != nil || code != 0 {
		return fmt.Errorf("compile native fixture: %v exit=%d %s", err, code, out)
	}
	binary, err := os.ReadFile("/tmp/rust-test-fixture/release/niri-tonic-files")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(archive), 0755); err != nil {
		return err
	}
	f, err := os.Create(archive)
	if err != nil {
		return err
	}
	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)
	if err := tw.WriteHeader(&tar.Header{Name: "bin/niri-tonic-files", Mode: 0755, Size: int64(len(binary))}); err != nil {
		return err
	}
	if _, err := tw.Write(binary); err != nil {
		return err
	}
	if err := tw.Close(); err != nil {
		return err
	}
	if err := gz.Close(); err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	data, err := os.ReadFile(archive)
	if err != nil {
		return err
	}
	sum := fmt.Sprintf("%x", sha256.Sum256(data))
	receipt := map[string]any{"schema_version": 1, "source_sha": source, "version": "1.0.0", "component": "niri-tonic-files", "arch": runtime.GOARCH, "archive": filepath.Base(archive), "sha256": sum, "run_id": "123", "run_attempt": "1", "tested": false}
	if err := catalog.Write(archive+".receipt.json", receipt); err != nil {
		return err
	}
	args := append(base, "--archive="+archive, "--receipt="+archive+".receipt.json", "--output=/reports/qualified")
	if err := run("exact archive Rust and headless Wayland qualification", args, true, "synthetic smoke passed"); err != nil {
		return err
	}
	var qualified struct {
		Tested bool
		SHA256 string
	}
	if err := catalog.Read("/reports/qualified/"+filepath.Base(archive)+".receipt.json", &qualified); err != nil {
		return err
	}
	copy, err := os.ReadFile("/reports/qualified/" + filepath.Base(archive))
	if err != nil {
		return err
	}
	if !qualified.Tested || qualified.SHA256 != sum || fmt.Sprintf("%x", sha256.Sum256(copy)) != sum {
		return fmt.Errorf("qualification changed archive bytes or lost tested receipt")
	}
	if err := os.WriteFile(archive, []byte("corrupted"), 0644); err != nil {
		return err
	}
	return run("reject corrupted binary archive", append(args, "--output=/reports/rejected-qualified"), false, "candidate checksum mismatch")
}
