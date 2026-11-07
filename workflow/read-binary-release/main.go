package main

import (
	"fmt"
	"github.com/tdesaules/github-actions/shared/binaryrelease"
	"os"
)

func main() {
	if e := binaryrelease.RunCLI("read-binary-release 1", os.Args[1:], []string{"tag", "repository", "output"}, os.Stdout, binaryrelease.ReadAction); e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
}
