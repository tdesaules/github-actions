package main

import "github.com/tdesaules/github-actions/shared/testsuite"

func main() { testsuite.Main("python3", "-B", "-m", "unittest", "discover", "-s", "tests/unit", "-v") }
