package component

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestIsolatedRegistryCredentials(t *testing.T) {
	file, cleanup, err := auth("registry.example:5000", "fixture", "private-fixture-token")
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(file)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatal("auth file is not private", err)
	}
	data, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	var config struct {
		Auths map[string]any `json:"auths"`
	}
	if err := json.Unmarshal(data, &config); err != nil {
		t.Fatal(err)
	}
	if len(config.Auths) != 1 || config.Auths["registry.example:5000"] == nil || strings.Contains(string(data), "private-fixture-token") {
		t.Fatal("incorrect credential scope")
	}
	cleanup()
	if _, err := os.Stat(file); !os.IsNotExist(err) {
		t.Fatal("auth file survived cleanup")
	}
}
func TestRejectMutableReferencesBeforeNetworkOrOutput(t *testing.T) {
	for _, reference := range []string{"ghcr.io/example/component:latest", "http://registry/component", "ghcr.io/example/component@sha256:invalid"} {
		output := t.TempDir() + "/output"
		if _, _, err := Fetch(reference, output, "", ""); err == nil {
			t.Fatal("invalid reference accepted")
		}
		if _, err := os.Stat(output); !os.IsNotExist(err) {
			t.Fatal("failed validation created output")
		}
	}
}
