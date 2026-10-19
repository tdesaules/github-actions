// Package testsuite launches a fixed test tool without a shell dispatcher.
package testsuite

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"syscall"
)

func Main(binary string, args ...string) {
	flags := flag.NewFlagSet("test", flag.ExitOnError)
	directory := flags.String("directory", os.Getenv("INPUT_DIRECTORY"), "working directory")
	version := flags.Bool("tool-version", false, "print the tool version")
	flags.Parse(os.Args[1:])
	if flags.NArg() != 0 {
		fail(fmt.Errorf("unexpected positional arguments"))
	}
	if *version {
		if binary == "go" {
			args = []string{"version"}
		} else {
			args = []string{"--version"}
		}
	} else {
		if *directory == "" {
			*directory = "."
		}
		if err := os.Chdir(*directory); err != nil {
			fail(err)
		}
	}
	path, err := exec.LookPath(binary)
	if err != nil {
		fail(err)
	}
	// Replace PID 1 so cancellation and the tool's exit status reach the runner.
	fail(syscall.Exec(path, append([]string{binary}, args...), os.Environ()))
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
