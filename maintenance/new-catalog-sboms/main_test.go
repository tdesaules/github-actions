package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tdesaules/github-actions/shared/catalog"
)

func TestUnpublishedImageDoesNotBlockPublishedSBOMs(t *testing.T) {
	root := t.TempDir()
	bin := filepath.Join(root, "syft")
	log := filepath.Join(root, "calls")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\nprintf '%s\\n' \"$*\" >> \"$SYFT_TEST_LOG\"\n"), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", root+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("SYFT_TEST_LOG", log)
	digest := "ghcr.io/fixture/tool@sha256:" + strings.Repeat("a", 64)
	images := []catalog.Image{
		{Name: "unpublished", Reference: ""},
		{Name: "published", Reference: digest},
	}
	if err := generate(images, filepath.Join(root, "reports")); err != nil {
		t.Fatal(err)
	}
	calls, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(calls), "registry:"+digest) != 2 ||
		!strings.Contains(string(calls), "--platform linux/amd64") ||
		!strings.Contains(string(calls), "--platform linux/arm64") {
		t.Fatalf("both published variants must be attempted: %s", calls)
	}
	var report map[string]string
	if err := catalog.Read(filepath.Join(root, "reports/unpublished-images.json"), &report); err != nil {
		t.Fatal(err)
	}
	if report["unpublished"] != "not_run: no published digest" {
		t.Fatal(report)
	}
}
