package main

import (
	"fmt"
	"github.com/tdesaules/github-actions/shared/binaryrelease"
	"os"
)

func main() {
	if e := binaryrelease.RunCLI("publish-binary-release 1", os.Args[1:], []string{"repository", "tag", "qualified-directory"}, os.Stdout, binaryrelease.PublishAction); e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
}
