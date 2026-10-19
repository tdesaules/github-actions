package main

import "github.com/tdesaules/github-actions/shared/testsuite"

func main() { testsuite.Main("go", "test", "./...") }
