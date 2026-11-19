package main

import (
	"github.com/tdesaules/github-actions/shared/chezmoi"
	"github.com/tdesaules/github-actions/shared/workflow"
)

func run() error { return chezmoi.Run("python3", ".github/scripts/plan_ci.py") }
func main()      { workflow.Main("workflow-plan-chezmoi 1", run) }
