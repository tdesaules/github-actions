package main

import (
	"fmt"
	"github.com/tdesaules/github-actions/shared/binaryrelease"
	"os"
)

func main() {
	if e := binaryrelease.RunCLI("plan-rust-gtk-release 1", os.Args[1:], []string{"directory", "event", "ref", "before", "source-sha", "mode", "initial-release", "output", "config"}, os.Stdout, binaryrelease.PlanAction); e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
}
