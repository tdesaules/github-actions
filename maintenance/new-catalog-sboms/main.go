package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/tdesaules/github-actions/shared/catalog"
	"github.com/tdesaules/github-actions/shared/workflow"
)

func run() error {
	output := flag.String("output", workflow.Default("INPUT_OUTPUT", ".build/maintenance"), "report directory")
	flag.Parse()
	if flag.NArg() != 0 {
		return fmt.Errorf("unexpected arguments")
	}
	if err := os.MkdirAll(*output, 0755); err != nil {
		return err
	}
	images, err := catalog.Discover()
	if err != nil {
		return err
	}
	failures := []string{}
	for _, image := range images {
		if !catalog.Pinned.MatchString(image.Reference) {
			return fmt.Errorf("%s: maintenance requires a published digest", image.Name)
		}
	}
	os.Setenv("SYFT_GOLANG_CAPTURE_SYMBOLS", "all")
	os.Setenv("SYFT_FILE_METADATA_SELECTION", "all")
	for _, image := range images {
		if !catalog.Pinned.MatchString(image.Reference) {
			return fmt.Errorf("%s: maintenance requires a published digest", image.Name)
		}
		for _, arch := range []string{"amd64", "arm64"} {
			dir := filepath.Join(*output, image.Name, arch)
			if err := os.MkdirAll(dir, 0755); err != nil {
				return err
			}
			for _, name := range []string{"sbom.spdx.json", "sbom.syft.json"} {
				if err := os.Remove(filepath.Join(dir, name)); err != nil && !os.IsNotExist(err) {
					return err
				}
			}
			if err := catalog.Run("syft", "scan", "registry:"+image.Reference, "--platform", "linux/"+arch, "-o", "spdx-json="+filepath.Join(dir, "sbom.spdx.json"), "-o", "syft-json="+filepath.Join(dir, "sbom.syft.json")); err != nil {
				failures = append(failures, image.Name+"/"+arch)
			}
		}
	}
	if len(failures) > 0 {
		return fmt.Errorf("SBOM generation failed: %s", strings.Join(failures, ", "))
	}
	return nil
}
func main() { workflow.Main("new-catalog-sboms 1", run) }
