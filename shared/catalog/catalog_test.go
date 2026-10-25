package catalog

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReportsRemainReadableAcrossContainerUsers(t *testing.T) {
	path := filepath.Join(t.TempDir(), "reports", "publication.json")
	for _, version := range []string{"first", "replacement"} {
		if err := Write(path, map[string]string{"version": version}); err != nil {
			t.Fatal(err)
		}
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0644 {
			t.Fatalf("artifact mode is %o, expected 0644", info.Mode().Perm())
		}
		var data map[string]string
		if err = Read(path, &data); err != nil {
			t.Fatal(err)
		}
		if data["version"] != version {
			t.Fatal("replacement was not atomic")
		}
	}
}
