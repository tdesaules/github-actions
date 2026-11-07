package main

import (
	"fmt"
	"github.com/tdesaules/github-actions/shared/rustgtk"
	"github.com/tdesaules/github-actions/shared/workflow"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

func build(v rustgtk.Inputs) error {
	if e := rustgtk.Identity(v); e != nil {
		return e
	}
	if e := rustgtk.Source(v.SourceSHA); e != nil {
		return e
	}
	id, attempt := os.Getenv("GITHUB_RUN_ID"), os.Getenv("GITHUB_RUN_ATTEMPT")
	if id == "" || attempt == "" {
		return fmt.Errorf("GITHUB_RUN_ID and GITHUB_RUN_ATTEMPT required")
	}
	stamp, e := workflow.Git("show", "-s", "--format=%ct", v.SourceSHA)
	if e != nil {
		return e
	}
	epoch, e := strconv.ParseInt(strings.TrimSpace(stamp), 10, 64)
	if e != nil {
		return e
	}
	target, e := os.MkdirTemp("", "rust-gtk-build-")
	if e != nil {
		return e
	}
	defer os.RemoveAll(target)
	env := []string{"NIRI_TONIC_RELEASE_VERSION=" + v.Version, "SOURCE_DATE_EPOCH=" + strings.TrimSpace(stamp), "CARGO_TARGET_DIR=" + target}
	if e = rustgtk.Run("", env, "cargo", "build", "--release", "--locked", "-j2", "--bin", rustgtk.Component); e != nil {
		return e
	}
	if e = rustgtk.Source(v.SourceSHA); e != nil {
		return e
	}
	archive := filepath.Join(v.Output, rustgtk.ArchiveName(v))
	if e = rustgtk.Package(v.Directory, filepath.Join(target, "release", rustgtk.Component), archive, epoch); e != nil {
		return e
	}
	hash, e := rustgtk.Hash(archive)
	if e != nil {
		return e
	}
	if e = rustgtk.Source(v.SourceSHA); e != nil {
		return e
	}
	r := rustgtk.Receipt{SchemaVersion: 1, SourceSHA: v.SourceSHA, Version: v.Version, Component: rustgtk.Component, Arch: v.Arch, Archive: filepath.Base(archive), SHA256: hash, RunID: id, RunAttempt: attempt}
	receipt := archive + ".receipt.json"
	if e = rustgtk.JSON(receipt, r); e != nil {
		return e
	}
	return rustgtk.Outputs(archive, receipt, hash)
}
func run() error {
	v, e := rustgtk.Flags("build")
	if e != nil {
		return e
	}
	if e = rustgtk.Paths(&v); e != nil {
		return e
	}
	return build(v)
}
func main() { rustgtk.Main(run) }
