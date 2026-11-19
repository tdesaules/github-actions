package main

import (
	"github.com/tdesaules/github-actions/shared/chezmoi"
	"github.com/tdesaules/github-actions/shared/workflow"
)

func run() error { return chezmoi.Run("python3", ".github/scripts/prepare_source_release.py") }
func main()      { workflow.Main("workflow-prepare-chezmoi-release 1", run) }
