package main

import "github.com/tdesaules/github-actions/shared/check"

func main() {
	check.Main("shellcheck 0.11.0", check.External("/bin/shellcheck"))
}
