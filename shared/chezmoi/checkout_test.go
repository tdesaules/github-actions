package chezmoi

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCheckoutBoundaryAndFailurePropagation(t *testing.T) {
	args := os.Args
	os.Args = []string{"entrypoint"}
	t.Cleanup(func() { os.Args = args })
	root := t.TempDir()
	t.Chdir(root)
	if err := os.Mkdir("source with spaces", 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile("source with spaces/marker", nil, 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Dir(root), "outside"); err != nil {
		t.Fatal(err)
	}
	for _, dir := range []string{"outside", "../", root, "missing"} {
		t.Setenv("INPUT_DIRECTORY", dir)
		if err := Run("true"); err == nil {
			t.Fatalf("accepted %q", dir)
		}
	}
	t.Setenv("INPUT_DIRECTORY", "source with spaces")
	if err := Run("test", "-f", "marker"); err != nil {
		t.Fatal(err)
	}
	if err := Run("false"); err == nil {
		t.Fatal("discarded helper failure")
	}
}
