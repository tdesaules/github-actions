package pantavisor

import "github.com/tdesaules/github-actions/shared/workflow"

func outputReference(name, value string) error { return workflow.Output(name, value) }
