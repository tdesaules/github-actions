package main

import "github.com/tdesaules/github-actions/shared/check"

func main() {
	check.Main("actionlint 1.7.7", check.External("/usr/bin/actionlint", "-shellcheck=", "-pyflakes="))
}
