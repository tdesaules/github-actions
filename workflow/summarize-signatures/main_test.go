package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSigningOutcomes(t *testing.T) {
	for _, test := range []struct {
		outcome string
		signed  bool
		want    string
	}{
		{"success", true, "signed"}, {"success", false, "not_run"},
		{"failure", false, "error"}, {"skipped", false, "not_run"}, {"cancelled", false, "not_run"},
	} {
		if got := signing(test.outcome, test.signed); got.Status != test.want {
			t.Fatalf("%+v: %+v", test, got)
		}
	}
}

func TestVerificationRequiresBoundEvidence(t *testing.T) {
	subject := "ghcr.io/owner/image@sha256:" + strings.Repeat("a", 64)
	path := filepath.Join(t.TempDir(), "cosign.json")
	if got := verification("success", path, subject); got.Status != "error" {
		t.Fatal("missing report was treated as verified")
	}
	data := fmt.Sprintf(`[{"critical":{"type":"cosign container image signature","image":{"docker-manifest-digest":"sha256:%s"},"identity":{"docker-reference":"ghcr.io/owner/image"}}}]`, strings.Repeat("a", 64))
	if err := os.WriteFile(path, []byte(data), 0644); err != nil {
		t.Fatal(err)
	}
	if got := verification("success", path, subject); got.Status != "verified" {
		t.Fatal(got)
	}
	if got := verification("success", path, "ghcr.io/other/image@sha256:"+strings.Repeat("a", 64)); got.Status != "error" {
		t.Fatal("wrong repository was treated as verified")
	}
	if got := verification("skipped", path, subject); got.Status != "not_run" {
		t.Fatal("a stale report must not override the actual skipped outcome")
	}
}
