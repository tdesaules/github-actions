package main

import (
	"flag"
	"fmt"
	"github.com/tdesaules/github-actions/shared/buildreceipt"
	"github.com/tdesaules/github-actions/shared/workflow"
	"os"
)

func run() error {
	archive := flag.String("archive", os.Getenv("INPUT_ARCHIVE"), "downloaded archive")
	receipt := flag.String("receipt", os.Getenv("INPUT_RECEIPT"), "build receipt")
	image := flag.String("image", os.Getenv("INPUT_IMAGE"), "expected image")
	arch := flag.String("architecture", os.Getenv("INPUT_ARCHITECTURE"), "expected architecture")
	revision := flag.String("revision", os.Getenv("INPUT_REVISION"), "expected revision")
	version := flag.String("version", os.Getenv("INPUT_VERSION"), "expected candidate version")
	flag.Parse()
	if flag.NArg() != 0 || *archive == "" || *receipt == "" {
		return fmt.Errorf("archive and receipt are required")
	}
	sum, err := buildreceipt.Verify(*archive, *receipt, buildreceipt.Receipt{Image: *image, Arch: *arch, Revision: *revision, Version: *version})
	if err != nil {
		return err
	}
	if err = workflow.Output("archive-sha256", sum); err != nil {
		return err
	}
	return workflow.Output("verified", true)
}
func main() { workflow.Main("test-build-artifact 1", run) }
