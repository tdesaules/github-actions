package main

import "github.com/tdesaules/github-actions/shared/check"

func main() {
	check.Main("markdownlint 0.41.1", func(path string) error {
		return check.External("/usr/local/bin/node", "/opt/markdown/check.mjs")(path)
	})
}
