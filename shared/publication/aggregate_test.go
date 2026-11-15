package publication

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tdesaules/github-actions/shared/catalog"
)

func TestScopedPublicationCannotFinalizeCatalogue(t *testing.T) {
	directory, _ := fixture(t)
	if err := Prepare(directory, "fixture", filepath.Join(directory, "release-plan.json"), "check-test-fixture"); err != nil {
		t.Fatal(err)
	}
	state, err := Load(directory)
	if err != nil || state.Image != "check-test-fixture" || len(state.Images) != 1 {
		t.Fatalf("invalid scoped plan: %+v %v", state, err)
	}
	if err := Finish(directory); err == nil || !strings.Contains(err.Error(), "aggregated") {
		t.Fatalf("scoped receipt authorized catalogue finalization: %v", err)
	}
}

func TestAggregateRequiresCompleteVerifiedReceipts(t *testing.T) {
	for _, scenario := range []string{"valid", "missing", "duplicate", "stale", "unpublished", "wrong-platform", "remote-mismatch"} {
		t.Run(scenario, func(t *testing.T) {
			directory, revision := fixture(t)
			child := `{"schemaVersion":2}`
			digest := Hash([]byte(child))
			index := fmt.Sprintf(`{"schemaVersion":2,"manifests":[{"digest":%q,"platform":{"os":"linux","architecture":"amd64"}},{"digest":%q,"platform":{"os":"linux","architecture":"arm64"}}]}`, digest, digest)
			reference := "ghcr.io/fixture/github-actions/check-test-fixture@" + Hash([]byte(index))
			state := State{Image: "check-test-fixture", RunID: os.Getenv("GITHUB_RUN_ID"), Revision: revision, Version: "0.1.0",
				Images: map[string]*Image{"check-test-fixture": {
					Reference: reference, Platforms: map[string]string{"amd64": digest, "arm64": digest}, Published: true,
					Archives: map[string]Archive{
						"amd64": {Path: "amd64/image.oci.tar", SHA256: strings.Repeat("a", 64)},
						"arm64": {Path: "arm64/image.oci.tar", SHA256: strings.Repeat("b", 64)},
					},
				}}}
			switch scenario {
			case "stale":
				state.Revision = strings.Repeat("0", 40)
			case "unpublished":
				state.Images[state.Image].Published = false
			case "wrong-platform":
				state.Images[state.Image].Platforms["arm64"] = "sha256:" + strings.Repeat("0", 64)
			}
			tools := t.TempDir()
			if scenario == "remote-mismatch" {
				index = "{}"
			}
			script := "#!/bin/sh\ncase \"$3\" in\n*@" + digest + ") printf '%s' '" + child + "';;\n*) printf '%s' '" + index + "';;\nesac\n"
			if err := os.WriteFile(filepath.Join(tools, "skopeo"), []byte(script), 0755); err != nil {
				t.Fatal(err)
			}
			t.Setenv("PATH", tools+string(os.PathListSeparator)+os.Getenv("PATH"))
			receipts := ".build/receipts"
			if err := os.MkdirAll(receipts, 0755); err != nil {
				t.Fatal(err)
			}
			if scenario != "missing" {
				if err := Save(filepath.Join(receipts, "one"), state); err != nil {
					t.Fatal(err)
				}
			}
			if scenario == "duplicate" {
				if err := Save(filepath.Join(receipts, "two"), state); err != nil {
					t.Fatal(err)
				}
			}
			err := Aggregate(".build/aggregate", receipts, filepath.Join(directory, "release-plan.json"))
			if (err == nil) != (scenario == "valid") {
				t.Fatalf("%s: %v", scenario, err)
			}
			if err == nil {
				var merged State
				if err := catalog.Read(".build/aggregate/publication.json", &merged); err != nil || merged.Image != "" || len(merged.Images) != 1 {
					t.Fatalf("invalid aggregate: %+v %v", merged, err)
				}
			}
		})
	}
}
