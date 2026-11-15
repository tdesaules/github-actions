package main

import (
	"fmt"
	"os"

	"github.com/tdesaules/github-actions/shared/atomicrelease"
)

func main() {
	if err := atomicrelease.CLI("test-atomic-release", os.Args[1:], atomicrelease.VerifyAction); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
