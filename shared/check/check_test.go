package check

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExplicitFilesOverrideEnvironmentAndRetainSpaces(t *testing.T) {
	path := filepath.Join(t.TempDir(), "-file with spaces; literal")
	if err := os.WriteFile(path, []byte("source"), 0600); err != nil {
		t.Fatal(err)
	}
	value, _ := json.Marshal([]string{path})
	t.Setenv("INPUT_FILES", `["missing"]`)
	t.Setenv("GITHUB_OUTPUT", "")
	if err := run([]string{"--files=" + string(value)}, func(got string) error {
		if got != path {
			t.Fatalf("modified path: %s", got)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"--files=[]"}, {"--files=invalid"}, {"test-yaml"}, {"--unknown=true"}} {
		if err := run(args, func(string) error { t.Fatal("must not run validator"); return nil }); err == nil {
			t.Fatal("accepted invalid invocation")
		}
	}
}

func TestChecksContinueAfterFailureAndWriteFailureOutput(t *testing.T) {
	dir := t.TempDir()
	first, second, output := filepath.Join(dir, "first"), filepath.Join(dir, "second"), filepath.Join(dir, "output")
	for _, path := range []string{first, second, output} {
		if err := os.WriteFile(path, nil, 0600); err != nil {
			t.Fatal(err)
		}
	}
	value, _ := json.Marshal([]string{first, second})
	t.Setenv("GITHUB_OUTPUT", output)
	calls := 0
	err := run([]string{"--files=" + string(value)}, func(string) error { calls++; return errors.New("invalid fixture") })
	if err == nil || calls != 2 {
		t.Fatalf("expected both files and a failure, calls=%d err=%v", calls, err)
	}
	data, _ := os.ReadFile(output)
	if !strings.Contains(string(data), "checked=2\npassed=false\n") {
		t.Fatalf("incorrect outputs: %s", data)
	}
}
