package testsuite

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestPythonDiscovery(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)
	for _, directory := range []string{"tests/unit", "tests", "tests with spaces"} {
		if err := os.MkdirAll(directory, 0755); err != nil {
			t.Fatal(err)
		}
		got, err := pythonArgs(directory)
		want := []string{"-B", "-m", "unittest", "discover", "-s", directory, "-v"}
		if err != nil || !reflect.DeepEqual(got, want) {
			t.Fatalf("%q: got %v, %v", directory, got, err)
		}
	}
	for _, directory := range []string{"", "missing", "../outside", root, "tests\ninvalid"} {
		if _, err := pythonArgs(directory); err == nil {
			t.Errorf("accepted invalid test directory %q", directory)
		}
	}
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, "external")); err != nil {
		t.Fatal(err)
	}
	if _, err := pythonArgs("external"); err == nil {
		t.Fatal("accepted symlink outside project")
	}
}
