package main

import "github.com/tdesaules/github-actions/shared/check"

func main() {
	check.Main("hadolint 2.14.0", check.External("/bin/hadolint"))
}
