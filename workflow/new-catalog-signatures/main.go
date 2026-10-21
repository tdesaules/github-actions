package main

import (
	"flag"
	"fmt"
	"github.com/tdesaules/github-actions/shared/publication"
	"github.com/tdesaules/github-actions/shared/workflow"
)

func run() error {
	directory := flag.String("directory", workflow.Default("INPUT_DIRECTORY", ".build/publication"), "publication plan directory")
	flag.Parse()
	if flag.NArg() != 0 {
		return fmt.Errorf("unexpected arguments")
	}
	return publication.Sign(*directory)
}
func main() { workflow.Main("new-catalog-signatures 1", run) }
