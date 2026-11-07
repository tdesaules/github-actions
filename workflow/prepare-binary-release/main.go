package main

import (
	"fmt"
	"github.com/tdesaules/github-actions/shared/binaryrelease"
	"os"
)

func main() {
	if e := binaryrelease.RunCLI("prepare-binary-release 1", os.Args[1:], []string{"source-sha", "plan", "qualified-directory", "repository", "config"}, os.Stdout, binaryrelease.PrepareAction); e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
}
