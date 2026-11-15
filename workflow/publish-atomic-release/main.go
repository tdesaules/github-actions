package main

import (
	"fmt"
	"os"

	"github.com/tdesaules/github-actions/shared/atomicrelease"
)

func main() {
	if err := atomicrelease.CLI("publish-atomic-release", os.Args[1:], atomicrelease.PublishAction); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
