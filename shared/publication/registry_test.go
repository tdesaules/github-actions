package publication

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// Re-execute the test binary as Skopeo: the published Go action has no shell.
func TestMain(m *testing.M) {
	if response := os.Getenv("PUBLICATION_TEST_REGISTRY_RESPONSE"); response != "" {
		if len(os.Args) != 4 || os.Args[1] != "inspect" || os.Args[2] != "--raw" {
			fmt.Fprintln(os.Stderr, "unexpected registry inspection arguments")
			os.Exit(2)
		}
		if response == "present" {
			fmt.Print("{}")
			os.Exit(0)
		}
		fmt.Fprintln(os.Stderr, response)
		os.Exit(1)
	}
	os.Exit(m.Run())
}

func TestRegistryMissingIsNotAnyFailure(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, response  string
		missing, failed bool
	}{
		{"present", "present", false, false},
		{"missing", "manifest unknown", true, false},
		{"unauthorized", "unauthorized", false, true},
		{"network", "TLS handshake timeout", false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.Symlink(executable, filepath.Join(dir, "skopeo")); err != nil {
				t.Fatal(err)
			}
			t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
			t.Setenv("PUBLICATION_TEST_REGISTRY_RESPONSE", tc.response)
			data, err := rawOptional("example.invalid/catalog@sha256:fixture")
			if (err != nil) != tc.failed {
				t.Fatal(err)
			}
			if err == nil && (data == nil) != tc.missing {
				t.Fatalf("unexpected registry result %q", data)
			}
		})
	}
}
