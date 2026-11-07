package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tdesaules/github-actions/shared/candidate"
	"github.com/tdesaules/github-actions/shared/fixtures"
)

// Run the compiled test binary only in a disposable candidate image, without
// network access. Hosted qualification still tests the original exact OCI archive.
func TestDisposableFixtureCases(t *testing.T) {
	image := os.Getenv("CANDIDATE_FIXTURE_IMAGE")
	if image == "" {
		t.Skip("requires disposable candidate container")
	}
	if os.Geteuid() != 0 {
		t.Fatal("requires container root")
	}
	for _, name := range []string{"/reports", "/fixtures"} {
		if err := os.MkdirAll(name, 0755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Chmod("/reports", 0777); err != nil {
		t.Fatal(err)
	}
	root := &candidate.Root{Path: "/"}
	root.Config.Config.Env = os.Environ()
	results := []result{}
	if strings.Contains(image, "binary-release") || image == "workflow-plan-rust-gtk-release" {
		if err := binaryCases(root, image, &results); err != nil {
			t.Fatal(err)
		}
	} else {
		source, err := fixtures.RustRepository("/fixtures/rust-app with spaces")
		if err != nil {
			t.Fatal(err)
		}
		if err := filepath.WalkDir("/fixtures", func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				return os.Chmod(path, 0555)
			}
			return os.Chmod(path, 0444)
		}); err != nil {
			t.Fatal(err)
		}
		if err := rustCases(root, image, source, &results); err != nil {
			t.Fatal(err)
		}
	}
	for _, r := range results {
		t.Logf("%s: passed=%t exit=%d", r.Name, r.Passed, r.Exit)
	}
}
