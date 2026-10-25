package main

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/tdesaules/github-actions/shared/candidate"
	"github.com/tdesaules/github-actions/shared/catalog"
	"github.com/tdesaules/github-actions/shared/fixtures"
	"github.com/tdesaules/github-actions/shared/workflow"
)

type result struct {
	Name   string `json:"name"`
	Passed bool   `json:"passed"`
	Exit   int    `json:"exit"`
	Output string `json:"output"`
}

func qualify(root *candidate.Root, image string, results *[]result) error {
	files := fixtures.Files()
	before := map[string][32]byte{}
	for name, data := range files {
		p := filepath.Join(root.Path, "fixtures", name)
		if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
			return err
		}
		if err := os.WriteFile(p, []byte(data), 0644); err != nil {
			return err
		}
		before[name] = sha256.Sum256([]byte(data))
	}
	if err := root.Protect(); err != nil {
		return err
	}
	if err := root.Install(); err != nil {
		return err
	}
	test := func(name, binary string, args []string, wantSuccess bool, diagnostic string) error {
		out, code, err := root.Run(binary, args...)
		passed := err == nil && (code == 0) == wantSuccess && (diagnostic == "" || strings.Contains(out, diagnostic))
		*results = append(*results, result{name, passed, code, out})
		if !passed {
			return fmt.Errorf("%s: exit=%d error=%v output=%s", name, code, err, out)
		}
		return nil
	}
	if err := test("native tool version", "/entrypoint", []string{"--tool-version"}, true, ""); err != nil {
		return err
	}
	if image == "workflow-start-test-registry" {
		err := service(root)
		*results = append(*results, result{Name: "native TLS registry lifecycle", Passed: err == nil})
		if err != nil {
			return err
		}
	}
	pairs := map[string][2]string{
		"check-test-github-workflow": {"workflow valid.yml", "workflow invalid.yml"}, "check-test-containerfile": {"Containerfile valid", "Containerfile invalid"},
		"check-test-shell": {"script without extension", "script invalid.script"}, "check-test-yaml": {"valid stream.yaml", "invalid.yaml"},
		"check-test-toml": {"valid config.toml", "invalid.toml"}, "check-test-markdown": {"valid document.md", "invalid document.md"},
		"check-test-links": {"valid links.md", "invalid links.md"}, "check-test-action-metadata": {"valid action.yml", "invalid action.yml"},
	}
	if pair, ok := pairs[image]; ok {
		for _, c := range []struct {
			names   []string
			success bool
		}{{[]string{pair[0]}, true}, {[]string{pair[1]}, false}, {[]string{pair[0], pair[1]}, false}, {[]string{}, false}, {[]string{"missing file"}, false}, {[]string{pair[0], pair[0]}, false}} {
			data, _ := json.Marshal(c.names)
			diagnostic := ""
			if image == "check-test-markdown" && len(c.names) == 1 && c.names[0] == pair[1] {
				diagnostic = "MD018"
			}
			if err := test("literal files "+string(data), "/entrypoint", []string{"--files=" + string(data)}, c.success, diagnostic); err != nil {
				return err
			}
		}
		unreadable := filepath.Join(root.Path, "fixtures", pair[0])
		if err := os.Chmod(unreadable, 0); err != nil {
			return err
		}
		data, _ := json.Marshal([]string{pair[0]})
		err := test("unreadable source", "/entrypoint", []string{"--files=" + string(data)}, false, "")
		restoreErr := os.Chmod(unreadable, 0444)
		if err != nil {
			return err
		}
		if restoreErr != nil {
			return restoreErr
		}
	}
	for component, prefix := range map[string]string{"check-test-go": "go", "check-test-python": "python", "check-test-catalog": "catalog"} {
		if image == component {
			root.Config.Config.Env = append(root.Config.Config.Env, "INPUT_DIRECTORY=missing")
			if err := test("valid read-only suite and CLI precedence", "/entrypoint", []string{"--directory=/fixtures/" + prefix + "-valid with spaces"}, true, ""); err != nil {
				return err
			}
			diagnostic := "intentional failure"
			if prefix == "catalog" {
				diagnostic = "unpinned base"
			}
			if err := test("genuine suite failure", "/entrypoint", []string{"--directory=/fixtures/" + prefix + "-invalid with spaces"}, false, diagnostic); err != nil {
				return err
			}
		}
	}
	if image == "oci-new-sbom" {
		if err := test("read-only SBOM source", "/entrypoint", []string{"--source=dir:/fixtures/fixture with spaces", "--output=/reports/sbom.json"}, true, ""); err != nil {
			return err
		}
		var document map[string]any
		if err := catalog.Read(filepath.Join(root.Path, "reports/sbom.json"), &document); err != nil {
			return err
		}
		version, _ := document["spdxVersion"].(string)
		if !strings.HasPrefix(version, "SPDX-") {
			return fmt.Errorf("missing SPDX report")
		}
		if err := test("missing SBOM source", "/entrypoint", []string{"--source=dir:/fixtures/missing", "--output=/reports/missing.json"}, false, ""); err != nil {
			return err
		}
		if err := test("unwritable SBOM output", "/entrypoint", []string{"--source=dir:/fixtures/fixture with spaces", "--output=/fixtures/unwritable.json"}, false, ""); err != nil {
			return err
		}
		if _, err := os.Stat(filepath.Join(root.Path, "fixtures/unwritable.json")); !os.IsNotExist(err) {
			return fmt.Errorf("permission failure produced an output")
		}
	}
	if image == "maintenance-update-dependencies" {
		if err := test("Renovate runtime", "/usr/local/renovate/node", []string{"/usr/local/renovate/dist/renovate.js", "--help"}, true, "Usage"); err != nil {
			return err
		}
	}
	for name, sum := range before {
		data, err := os.ReadFile(filepath.Join(root.Path, "fixtures", name))
		if err != nil {
			return err
		}
		if sha256.Sum256(data) != sum {
			return fmt.Errorf("candidate modified source %s", name)
		}
	}
	return nil
}

func run() (err error) {
	archive := flag.String("archive", os.Getenv("INPUT_ARCHIVE"), "OCI build archive; empty to test a published reference")
	reference := flag.String("reference", os.Getenv("INPUT_REFERENCE"), "published immutable image reference")
	image := flag.String("image", os.Getenv("INPUT_IMAGE"), "expected image owner")
	arch := flag.String("architecture", os.Getenv("INPUT_ARCHITECTURE"), "native architecture")
	revision := flag.String("revision", os.Getenv("INPUT_REVISION"), "optional expected build revision")
	output := flag.String("output", os.Getenv("INPUT_OUTPUT"), "runtime test report")
	flag.Parse()
	if flag.NArg() != 0 || *image == "" || *output == "" || (*arch != "amd64" && *arch != "arm64") {
		return fmt.Errorf("image, architecture and output are required")
	}
	if *archive != "" {
		source, err := filepath.Abs(*archive)
		if err != nil {
			return err
		}
		target, err := filepath.Abs(*output)
		if err != nil {
			return err
		}
		if source == target {
			return fmt.Errorf("runtime report must not overwrite the candidate archive")
		}
	}
	results := []result{}
	defer func() { err = errors.Join(err, catalog.Write(*output, results)) }()
	tmp, err := os.MkdirTemp("", "candidate-runtime-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	source := ""
	if *archive != "" {
		p, e := filepath.Abs(*archive)
		if e != nil {
			return e
		}
		source = "oci-archive:" + p
	} else {
		if !catalog.Pinned.MatchString(*reference) {
			return fmt.Errorf("a published digest or build archive is required")
		}
		source = "docker://" + *reference
	}
	layout := filepath.Join(tmp, "layout")
	if err = catalog.Run("/usr/bin/skopeo", "copy", "--preserve-digests", "--override-arch", *arch, source, "oci:"+layout+":candidate"); err != nil {
		return err
	}
	root, err := candidate.Open(layout, filepath.Join(tmp, "root"), *image, *arch, *revision)
	if err != nil {
		return err
	}
	return qualify(root, *image, &results)
}
func main() { workflow.Main("test-candidate-runtime 1", run) }
