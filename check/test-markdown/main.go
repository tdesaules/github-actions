package main

import "github.com/tdesaules/github-actions/shared/check"

func main() {
	check.Main("markdownlint-cli2 0.23.3", func(path string) error {
		return check.External("/usr/local/bin/node", "/usr/local/lib/node_modules/markdownlint-cli2/markdownlint-cli2-bin.mjs", "--no-globs")(":" + path)
	})
}
