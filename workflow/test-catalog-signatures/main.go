package main

import (
	"flag"
	"fmt"
	"github.com/tdesaules/github-actions/shared/publication"
	"github.com/tdesaules/github-actions/shared/workflow"
	"os"
)

func run() error {
	directory := flag.String("directory", workflow.Default("INPUT_DIRECTORY", ".build/publication"), "publication plan and signature directory")
	tag := flag.String("tag", os.Getenv("INPUT_TAG"), "optional release tag to verify against the checkout")
	repo := flag.String("repository", workflow.Default("INPUT_REPOSITORY", os.Getenv("GITHUB_REPOSITORY")), "signing repository")
	flag.Parse()
	if flag.NArg() != 0 {
		return fmt.Errorf("unexpected arguments")
	}
	return publication.Verify(*directory, *tag, *repo)
}
func main() { workflow.Main("test-catalog-signatures 1", run) }
