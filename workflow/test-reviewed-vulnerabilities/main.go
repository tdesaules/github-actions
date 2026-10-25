package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/tdesaules/github-actions/shared/catalog"
	"github.com/tdesaules/github-actions/shared/workflow"
)

func run() error {
	sbom := flag.String("sbom", os.Getenv("INPUT_SBOM"), "Syft SBOM")
	config := flag.String("config", os.Getenv("INPUT_CONFIG"), "evidence-derived Grype review")
	output := flag.String("output", os.Getenv("INPUT_OUTPUT"), "vulnerability report")
	metadata := flag.String("database-metadata", os.Getenv("INPUT_DATABASE-METADATA"), "database snapshot metadata")
	flag.Parse()
	if flag.NArg() != 0 || *sbom == "" || *config == "" || *output == "" || *metadata == "" {
		return fmt.Errorf("sbom, config, output and database-metadata are required")
	}
	for _, p := range []string{*sbom, *config} {
		info, err := os.Stat(p)
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("input must be a regular file")
		}
	}
	paths := map[string]bool{}
	for _, p := range []string{*sbom, *config, *output, *metadata} {
		absolute, err := filepath.Abs(p)
		if err != nil {
			return err
		}
		if paths[absolute] {
			return fmt.Errorf("input and output paths must be distinct")
		}
		paths[absolute] = true
	}
	for _, p := range []string{*output, *metadata} {
		if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
			return err
		}
	}
	cache, err := os.MkdirTemp("", "candidate-grype-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(cache)
	os.Setenv("GRYPE_DB_CACHE_DIR", cache)
	os.Setenv("GRYPE_DB_AUTO_UPDATE", "false")
	if err = catalog.Run("grype", "db", "update"); err != nil {
		return err
	}
	data, err := exec.Command("grype", "db", "status", "-o", "json").Output()
	if err != nil {
		return err
	}
	if err = os.WriteFile(*metadata, data, 0644); err != nil {
		return err
	}
	return catalog.Run("grype", "sbom:"+*sbom, "--config", *config, "--fail-on", "high", "-o", "json", "--file", *output)
}
func main() { workflow.Main("test-reviewed-vulnerabilities 1", run) }
