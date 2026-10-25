// Package fixtures contains literal test inputs, never a component catalogue.
package fixtures

import "strings"

func Files() map[string]string {
	files := map[string]string{
		"syft-empty.json":                                  "{\"artifacts\":[],\"files\":[]}",
		"review-empty.json":                                "{\"ignore\":[]}",
		"fixture with spaces/hello.txt":                    "hello\n",
		"fixture with spaces/Containerfile":                "FROM scratch\nCOPY hello.txt /hello.txt\nUSER 65532:65532\n",
		"workflow valid.yml":                               "name: fixture\non: push\njobs:\n  test:\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo hello\n",
		"workflow invalid.yml":                             "on: push\njobs:\n  test:\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo '${{ unknown.value }}'\n",
		"Containerfile valid":                              "FROM scratch\nCOPY hello /hello\nUSER 65532:65532\n",
		"Containerfile invalid":                            "FROM ubuntu:latest\nRUN apt-get update\n",
		"script without extension":                         "#!/bin/sh\nvalue='hello world'\nprintf '%s\\n' \"$value\"\n",
		"script invalid.script":                            "#!/bin/sh\necho $undefined_variable\n",
		"valid stream.yaml":                                "name: first\n---\nname: second\n",
		"invalid.yaml":                                     "name: first\nname: duplicate\n",
		"valid config.toml":                                "[package]\nname = 'hello world'\n",
		"invalid.toml":                                     "name = 'first'\nname = 'duplicate'\n",
		"valid document.md":                                "# Fixture\n\nValid Markdown.\n",
		"invalid document.md":                              "#Bad heading\n",
		"valid links.md":                                   "# Links\n\n[Local file](valid%20config.toml)\n",
		"invalid links.md":                                 "# Links\n\n[Missing file](missing.toml)\n",
		"valid action.yml":                                 "name: Fixture\ndescription: Compose a fixture\nruns:\n  using: composite\n  steps:\n    - uses: ./lint/yaml\n",
		"invalid action.yml":                               "name: Fixture\ndescription: Invalid host execution\nruns:\n  using: composite\n  steps:\n    - run: python3 script.py\n",
		"go-valid/go.mod":                                  "module example.invalid/fixture\n\ngo 1.27\n",
		"go-valid/fixture_test.go":                         "package fixture\nimport \"testing\"\nfunc TestFixture(t *testing.T) {}\n",
		"go-invalid/go.mod":                                "module example.invalid/fixture\n\ngo 1.27\n",
		"go-invalid/fixture_test.go":                       "package fixture\nimport \"testing\"\nfunc TestFixture(t *testing.T) { t.Fatal(\"intentional failure\") }\n",
		"python-valid/tests/unit/test_fixture.py":          "import unittest\nclass Fixture(unittest.TestCase):\n def test_fixture(self): self.assertTrue(True)\n",
		"python-invalid/tests/unit/test_fixture.py":        "import unittest\nclass Fixture(unittest.TestCase):\n def test_fixture(self): self.fail('intentional failure')\n",
		"catalog-valid/check/test-fixture/action.yml":      "runs:\n  image: docker://example.invalid/check-test-fixture:unpublished\n",
		"catalog-valid/check/test-fixture/main.go":         "package main\n",
		"catalog-valid/check/test-fixture/Containerfile":   "FROM scratch AS final\n",
		"catalog-invalid/check/test-fixture/action.yml":    "runs:\n  image: docker://example.invalid/check-test-fixture:unpublished\n",
		"catalog-invalid/check/test-fixture/main.go":       "package main\n",
		"catalog-invalid/check/test-fixture/Containerfile": "FROM alpine:latest AS final\n",
	}
	result := map[string]string{}
	for name, data := range files {
		for _, prefix := range []string{"go-valid", "go-invalid", "python-valid", "python-invalid", "catalog-valid", "catalog-invalid"} {
			if strings.HasPrefix(name, prefix+"/") {
				name = prefix + " with spaces/" + strings.TrimPrefix(name, prefix+"/")
				break
			}
		}
		result[name] = data
	}
	return result
}
