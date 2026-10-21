package review

import (
	"encoding/json"
	"os"
	"testing"
)

func TestPatchEvidenceFailsClosed(t *testing.T) {
	var sbom SBOM
	if err := json.Unmarshal([]byte(`{"artifacts":[{"id":"one","name":"braces","version":"3.0.3","type":"npm","locations":[{"path":"/node_modules/braces/package.json"}]}],"files":[{"location":{"path":"/node_modules/braces/lib/compile.js"},"digests":[{"algorithm":"sha256","value":"expected"}]}]}`), &sbom); err != nil {
		t.Fatal(err)
	}
	policy := Policy{Patched: &Patch{Package: "braces", Version: "3.0.3", Files: map[string]string{"lib/compile.js": "expected"}}}
	config, evidence := Evaluate(sbom, policy)
	if len(config.Ignore) != 1 || len(evidence) != 1 {
		t.Fatal("valid evidence rejected")
	}
	sbom.Files = append(sbom.Files, sbom.Files[0])
	config, _ = Evaluate(sbom, policy)
	if len(config.Ignore) != 0 {
		t.Fatal("ambiguous evidence accepted")
	}
	sbom.Files = sbom.Files[:1]
	sbom.Files[0].Digests[0].Value = "altered"
	config, _ = Evaluate(sbom, policy)
	if len(config.Ignore) != 0 {
		t.Fatal("altered evidence accepted")
	}
	sbom.Files[0].Digests[0].Value = "expected"
	sbom.Artifacts[0].Version = "3.0.2"
	config, _ = Evaluate(sbom, policy)
	if len(config.Ignore) != 0 {
		t.Fatal("wrong version accepted")
	}
}
func TestAllGoCopiesNeedSymbols(t *testing.T) {
	policy := Policy{Package: "module", Affected: "module/daemon"}
	a := Artifact{Name: "module", Version: "1"}
	a.Metadata.Symbols = map[string][]string{"module/client": {"f"}}
	sbom := SBOM{Artifacts: []Artifact{a}}
	config, _ := Evaluate(sbom, policy)
	if len(config.Ignore) != 1 {
		t.Fatal("client symbols rejected")
	}
	sbom.Artifacts = append(sbom.Artifacts, Artifact{Name: "module", Version: "2"})
	config, _ = Evaluate(sbom, policy)
	if len(config.Ignore) != 0 {
		t.Fatal("missing symbols accepted")
	}
	sbom.Artifacts = sbom.Artifacts[:1]
	sbom.Artifacts[0].Metadata.Symbols["module/daemon/sub"] = []string{"f"}
	config, _ = Evaluate(sbom, policy)
	if len(config.Ignore) != 0 {
		t.Fatal("affected package accepted")
	}
}
func TestRealSBOMParity(t *testing.T) {
	path := os.Getenv("REVIEW_TEST_SBOM")
	if path == "" {
		t.Skip("optional captured SBOM parity fixture")
	}
	var sbom SBOM
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(data, &sbom); err != nil {
		t.Fatal(err)
	}
	var policy Policy
	data, err = os.ReadFile("../../grype-review.json")
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(data, &policy); err != nil {
		t.Fatal(err)
	}
	config, _ := Evaluate(sbom, policy)
	var expected Configuration
	data, err = os.ReadFile(os.Getenv("REVIEW_TEST_CONFIG"))
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(data, &expected); err != nil {
		t.Fatal(err)
	}
	actualJSON, _ := json.Marshal(config)
	expectedJSON, _ := json.Marshal(expected)
	if string(actualJSON) != string(expectedJSON) {
		t.Fatalf("Go review differs from previously qualified review: %s != %s", actualJSON, expectedJSON)
	}
}
