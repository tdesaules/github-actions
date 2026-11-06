package publication

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSignatureStages(t *testing.T) {
	for _, scenario := range []string{"new", "reused", "unpublished", "verification-fails", "signing-fails"} {
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
			script += "if [ \"$1\" = sign ]; then token=; IFS= read -r token < \"$4\" || :; [ \"$token\" = fixture-identity ] || exit 42; fi\n"
			if scenario == "signing-fails" {
				script += "exit 1\n"
			}
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
			var signing map[string]signatureResult
			if data, readErr := os.ReadFile(filepath.Join(directory, "signing-status.json")); readErr != nil || json.Unmarshal(data, &signing) != nil {
				t.Fatal("missing per-digest signing outcomes", readErr)
			}
			signed := 0
			for _, result := range signing {
				if result.Status == "signed" {
					signed++
				}
			}
			wantSigned := 3
			if scenario == "reused" || scenario == "unpublished" || scenario == "signing-fails" {
				wantSigned = 0
			}
			if signed != wantSigned {
				t.Fatalf("signed=%d want=%d: %+v", signed, wantSigned, signing)
			}
			calls, _ := os.ReadFile(log)
			if strings.Contains(string(calls), "fixture-identity") {
				t.Fatal("token leaked into argv")
			}
			for _, line := range strings.Split(strings.TrimSpace(string(calls)), "\n") {
				fields := strings.Fields(line)
				if len(fields) > 3 && fields[0] == "sign" {
					if _, statErr := os.Stat(fields[3]); !os.IsNotExist(statErr) {
						t.Fatal("token retained after signing")
					}
				}
			}
			if scenario == "signing-fails" {
				if err == nil {
					t.Fatal("signing failure ignored")
				}
				return
			}
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
