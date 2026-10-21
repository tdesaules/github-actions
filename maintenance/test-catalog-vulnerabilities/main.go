package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/tdesaules/github-actions/shared/catalog"
	"github.com/tdesaules/github-actions/shared/review"
	"github.com/tdesaules/github-actions/shared/workflow"
)

func run() error {
	directory := flag.String("directory", workflow.Default("INPUT_DIRECTORY", ".build/maintenance"), "report directory")
	policyPath := flag.String("policy", workflow.Default("INPUT_POLICY", "grype-review.json"), "review policy")
	flag.Parse()
	if flag.NArg() != 0 {
		return fmt.Errorf("unexpected arguments")
	}
	var policy review.Policy
	if err := catalog.Read(*policyPath, &policy); err != nil {
		return err
	}
	images, err := catalog.Discover()
	if err != nil {
		return err
	}
	cache, err := os.MkdirTemp("", "catalog-grype-db-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(cache)
	os.Setenv("GRYPE_DB_CACHE_DIR", cache)
	os.Setenv("GRYPE_DB_AUTO_UPDATE", "false")
	if err := catalog.Run("grype", "db", "update"); err != nil {
		return err
	}
	status, err := exec.Command("grype", "db", "status", "-o", "json").Output()
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(*directory, "grype-db.json"), status, 0644); err != nil {
		return err
	}
	failures := []string{}
	for _, image := range images {
		for _, arch := range []string{"amd64", "arm64"} {
			dir := filepath.Join(*directory, image.Name, arch)
			if err := os.Remove(filepath.Join(dir, "grype.json")); err != nil && !os.IsNotExist(err) {
				return err
			}
			var sbom review.SBOM
			if err := catalog.Read(filepath.Join(dir, "sbom.syft.json"), &sbom); err != nil {
				fmt.Fprintln(os.Stderr, err)
				failures = append(failures, image.Name+"/"+arch)
				continue
			}
			config, evidence := review.Evaluate(sbom, policy)
			if err := catalog.Write(filepath.Join(dir, "grype-reviewed.json"), config); err != nil {
				return err
			}
			if err := catalog.Write(filepath.Join(dir, "review-evidence.json"), evidence); err != nil {
				return err
			}
			if err := catalog.Run("grype", "sbom:"+filepath.Join(dir, "sbom.syft.json"), "--config", filepath.Join(dir, "grype-reviewed.json"), "--fail-on", "high", "-o", "json", "--file", filepath.Join(dir, "grype.json")); err != nil {
				failures = append(failures, image.Name+"/"+arch)
			}
		}
	}
	if len(failures) > 0 {
		return fmt.Errorf("maintenance scans failed: %s", strings.Join(failures, ", "))
	}
	return nil
}
func main() { workflow.Main("test-catalog-vulnerabilities 1", run) }
