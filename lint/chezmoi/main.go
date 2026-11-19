package main

import (
	"github.com/tdesaules/github-actions/shared/chezmoi"
	"github.com/tdesaules/github-actions/shared/workflow"
)

func run() error { return chezmoi.Run("python3", ".github/scripts/lint_dotfiles.py") }
func main()      { workflow.Main("lint-chezmoi 1", run) }
