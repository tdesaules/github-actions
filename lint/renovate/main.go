package main

import (
	"github.com/tdesaules/github-actions/shared/chezmoi"
	"github.com/tdesaules/github-actions/shared/workflow"
)

func run() error {
	return chezmoi.Run("/usr/local/renovate/node", "/usr/local/renovate/dist/config-validator.js", "--strict", "renovate.json")
}
func main() { workflow.Main("lint-renovate 1", run) }
