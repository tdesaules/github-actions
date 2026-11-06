package main

import (
	"flag"
	"fmt"
	"github.com/tdesaules/github-actions/shared/publication"
	"github.com/tdesaules/github-actions/shared/workflow"
	"os"
)

func run() error {
	tag := flag.String("tag", os.Getenv("INPUT_TAG"), "release tag bound to checkout")
	flag.Parse()
	if flag.NArg() != 0 {
		return fmt.Errorf("unexpected arguments")
	}
	cleanup, err := publication.Auth()
	if err != nil {
		return err
	}
	defer cleanup()
	_, err = publication.ValidateTagCatalog(*tag)
	return err
}
func main() { workflow.Main("test-catalog-publication 1", run) }
