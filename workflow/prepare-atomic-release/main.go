package main

import (
	"fmt"
	"os"

	"github.com/tdesaules/github-actions/shared/atomicrelease"
)

func main() {
	if err := atomicrelease.CLI("prepare-atomic-release", os.Args[1:], atomicrelease.PrepareAction); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
