package main

import (
	"flag"
	"fmt"
	"github.com/tdesaules/github-actions/shared/buildreceipt"
	"github.com/tdesaules/github-actions/shared/workflow"
	"os"
	"path/filepath"
)

func run() error {
	archive := flag.String("archive", os.Getenv("INPUT_ARCHIVE"), "build archive")
	output := flag.String("output", os.Getenv("INPUT_OUTPUT"), "untested receipt output")
	expected := flag.String("expected-sha256", os.Getenv("INPUT_EXPECTED-SHA256"), "builder checksum")
	image := flag.String("image", os.Getenv("INPUT_IMAGE"), "image owner")
	arch := flag.String("architecture", os.Getenv("INPUT_ARCHITECTURE"), "native architecture")
	revision := flag.String("revision", os.Getenv("INPUT_REVISION"), "source revision")
	version := flag.String("version", os.Getenv("INPUT_VERSION"), "candidate version")
	flag.Parse()
	if flag.NArg() != 0 || *archive == "" || *output == "" {
		return fmt.Errorf("archive and output are required")
	}
	a, err := filepath.Abs(*archive)
	if err != nil {
		return err
	}
	b, err := filepath.Abs(*output)
	if err != nil {
		return err
	}
	if a == b {
		return fmt.Errorf("receipt must not replace the archive")
	}
	return buildreceipt.Write(*archive, *output, *expected, buildreceipt.Receipt{Image: *image, Arch: *arch, Revision: *revision, Version: *version})
}
func main() { workflow.Main("new-build-receipt 1", run) }
