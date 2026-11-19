package main

import (
	"github.com/tdesaules/github-actions/shared/chezmoi"
	"github.com/tdesaules/github-actions/shared/workflow"
)

func run() error { return chezmoi.Run("python3", ".github/scripts/qualify_dotfiles.py") }
func main()      { workflow.Main("check-test-chezmoi 1", run) }
