package main

import (
	"fmt"
	"os"
	"os/exec"
)

func command(dir, name string, args ...string) {
	c := exec.Command(name, args...)
	c.Dir = dir
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	if e := c.Run(); e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
}
func main() {
	if len(os.Args) == 2 && os.Args[1] == "--tool-version" {
		fmt.Println("test-helmfile 1")
		return
	}
	command(os.Getenv("HELMFILE_TEST_ROOT"), "python3", "tests/validate.py")
	command(os.Getenv("HELMFILE_TEST_ROOT"), "python3", "-m", "unittest", "discover", "-s", "tests", "-p", "test_*.py")
}
