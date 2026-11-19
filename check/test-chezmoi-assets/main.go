package main

import (
	"github.com/tdesaules/github-actions/shared/chezmoi"
	"github.com/tdesaules/github-actions/shared/workflow"
)

func run() error { return chezmoi.Run("python3", ".github/scripts/check_release_assets.py") }
func main()      { workflow.Main("check-test-chezmoi-assets 1", run) }
