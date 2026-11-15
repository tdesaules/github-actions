package main

import (
	"flag"
	"fmt"
	"github.com/tdesaules/github-actions/shared/publication"
	"github.com/tdesaules/github-actions/shared/workflow"
	"os"
	"strings"
)

func run() error {
	directory := flag.String("directory", workflow.Default("INPUT_DIRECTORY", ".build/publication"), "artifact directory")
	owner := flag.String("owner", os.Getenv("INPUT_OWNER"), "registry owner")
	plan := flag.String("release-plan", os.Getenv("INPUT_RELEASE-PLAN"), "release plan path; empty means full bootstrap")
	image := flag.String("image", os.Getenv("PUBLICATION_IMAGE"), "optional single-image publication")
	receipts := flag.String("receipts", os.Getenv("PUBLICATION_RECEIPTS"), "aggregate published image receipts")
	flag.Parse()
	if flag.NArg() != 0 {
		return fmt.Errorf("unexpected arguments")
	}
	if *receipts != "" {
		if *image != "" {
			return fmt.Errorf("cannot aggregate a scoped publication")
		}
		return publication.Aggregate(*directory, *receipts, *plan)
	}
	return publication.Prepare(*directory, strings.ToLower(*owner), *plan, *image)
}
func main() { workflow.Main("new-publication-plan 1", run) }
