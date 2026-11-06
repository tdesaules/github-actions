package publication

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTagCatalogChecksAreIndependentOfSignatures(t *testing.T) {
	for _, scenario := range []string{"unsigned", "wrong-tag", "off-main", "digest-mismatch", "missing-platform"} {
		t.Run(scenario, func(t *testing.T) {
			fixture(t)
			raw := `{"manifests":[{"digest":"sha256:` + strings.Repeat("a", 64) + `","platform":{"os":"linux","architecture":"amd64"}},{"digest":"sha256:` + strings.Repeat("b", 64) + `","platform":{"os":"linux","architecture":"arm64"}}]}`
			if scenario == "missing-platform" {
				raw = `{"manifests":[]}`
			}
			ref := "ghcr.io/fixture/github-actions/check-test-fixture@" + Hash([]byte(raw))
			if scenario == "digest-mismatch" {
				ref = "ghcr.io/fixture/github-actions/check-test-fixture@sha256:" + strings.Repeat("c", 64)
			}
			if err := os.WriteFile("check/test-fixture/action.yml", []byte("runs:\n  image: docker://"+ref+"\n"), 0644); err != nil {
				t.Fatal(err)
			}
			gitTest(t, "add", "check/test-fixture/action.yml")
			gitTest(t, "commit", "-m", "chore: pin fixture")
			gitTest(t, "tag", "0.1.0")
			gitTest(t, "update-ref", "refs/remotes/origin/main", "HEAD")
			if scenario == "wrong-tag" {
				gitTest(t, "commit", "--allow-empty", "-m", "fix: newer source")
			}
			if scenario == "off-main" {
				gitTest(t, "update-ref", "refs/remotes/origin/main", "HEAD^")
			}
			tools := t.TempDir()
			if err := os.WriteFile(filepath.Join(tools, "skopeo"), []byte("#!/bin/sh\nprintf '%s' '"+raw+"'\n"), 0755); err != nil {
				t.Fatal(err)
			}
			t.Setenv("PATH", tools+string(os.PathListSeparator)+os.Getenv("PATH"))
			state, err := ValidateTagCatalog("0.1.0")
			if (err == nil) != (scenario == "unsigned") {
				t.Fatalf("%s: %v", scenario, err)
			}
			if err == nil && state.Images["check-test-fixture"].Signed {
				t.Fatal("unsigned publication claimed signed")
			}
		})
	}
}
