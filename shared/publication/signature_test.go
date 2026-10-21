package publication

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSignatureStages(t *testing.T) {
	for _, scenario := range []string{"new", "reused", "unpublished", "verification-fails"} {
		t.Run(scenario, func(t *testing.T) {
			directory, _ := fixture(t)
			if err := Prepare(directory, "fixture", filepath.Join(directory, "release-plan.json")); err != nil {
				t.Fatal(err)
			}
			state, err := Load(directory)
			if err != nil {
				t.Fatal(err)
			}
			image := state.Images["check-test-fixture"]
			image.Published = scenario != "unpublished"
			if scenario == "reused" {
				image.Reused = true
				if err = os.WriteFile("check/test-fixture/action.yml", []byte("runs:\n  image: docker://"+image.Reference+"\n"), 0644); err != nil {
					t.Fatal(err)
				}
			}
			if err = Save(directory, state); err != nil {
				t.Fatal(err)
			}
			tools := t.TempDir()
			log := filepath.Join(tools, "calls")
			script := "#!/bin/sh\nprintf '%s\\n' \"$*\" >> \"$SIGNATURE_TEST_LOG\"\n"
			if scenario == "verification-fails" {
				script += "if [ \"$1\" = verify ]; then exit 1; fi\n"
			}
			script += "printf '[]\\n'\n"
			if err = os.WriteFile(filepath.Join(tools, "cosign"), []byte(script), 0755); err != nil {
				t.Fatal(err)
			}
			t.Setenv("PATH", tools+string(os.PathListSeparator)+os.Getenv("PATH"))
			t.Setenv("SIGNATURE_TEST_LOG", log)
			old := requestOIDCToken
			requestOIDCToken = func() (string, error) { return "fixture-identity", nil }
			t.Cleanup(func() { requestOIDCToken = old })
			err = Sign(directory)
			if scenario == "unpublished" {
				if err == nil {
					t.Fatal("unpublished image signed")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			data, _ := os.ReadFile(log)
			count := strings.Count(string(data), "sign --yes")
			if scenario == "reused" && count != 0 {
				t.Fatal("reused image was resigned")
			}
			if scenario != "reused" && count != 3 {
				t.Fatal("missing platform or index signature", string(data))
			}
			err = Verify(directory, "", "fixture/catalog")
			if (err != nil) != (scenario == "verification-fails") {
				t.Fatal(err)
			}
			state, loadErr := Load(directory)
			if loadErr != nil {
				t.Fatal(loadErr)
			}
			if state.Images["check-test-fixture"].Signed != (scenario != "verification-fails") {
				t.Fatal("incorrect signed journal state")
			}
		})
	}
}
