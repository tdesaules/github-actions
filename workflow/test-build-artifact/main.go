package main

import (
	"flag"
	"fmt"
	"github.com/tdesaules/github-actions/shared/buildreceipt"
	"github.com/tdesaules/github-actions/shared/catalog"
	"github.com/tdesaules/github-actions/shared/workflow"
	"os"
	"path/filepath"
	"regexp"
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
	if path := os.Getenv("QUALIFICATION"); path != "" {
		a, _ := filepath.Abs(path)
		for _, src := range []string{*archive, *receipt} {
			b, _ := filepath.Abs(src)
			if a == b {
				return fmt.Errorf("qualification must not overwrite source")
			}
		}
		type qualified struct {
			Image, Arch, Revision, Version string
			Digest                         string `json:"digest"`
			Sum                            string `json:"archive_sha256"`
			Tested                         bool   `json:"tested"`
		}
		q := qualified{*image, *arch, *revision, *version, os.Getenv("TESTED_DIGEST"), sum, true}
		if q.Digest == "" {
			var found qualified
			if err = catalog.Read(path, &found); err != nil {
				return err
			}
			q.Digest = found.Digest
			if q != found {
				return fmt.Errorf("qualification differs from archive or build identity")
			}
		}
		if !regexp.MustCompile(`^sha256:[a-f0-9]{64}$`).MatchString(q.Digest) {
			return fmt.Errorf("invalid candidate digest")
		}
		if os.Getenv("TESTED_DIGEST") != "" {
			if err = catalog.Write(path, q); err != nil {
				return err
			}
		}
		if err = workflow.Output("digest", q.Digest); err != nil {
			return err
		}
	}
	if err = workflow.Output("archive-sha256", sum); err != nil {
		return err
	}
	return workflow.Output("verified", true)
}
func main() { workflow.Main("test-build-artifact 1", run) }
