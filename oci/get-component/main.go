package main

import (
	"github.com/tdesaules/github-actions/shared/component"
	"github.com/tdesaules/github-actions/shared/pantavisor"
	"github.com/tdesaules/github-actions/shared/workflow"
	"os"
)

func run() error {
	reference, output, user := pantavisor.Input("reference", ""), pantavisor.Input("output", ""), pantavisor.Input("registry-username", "")
	if err := pantavisor.Parse(); err != nil {
		return err
	}
	if err := pantavisor.Local(*output); err != nil {
		return err
	}
	_, digest, err := component.Fetch(*reference, *output, *user, os.Getenv("INPUT_REGISTRY-PASSWORD"))
	if err != nil {
		return err
	}
	if err := workflow.Output("component", *output); err != nil {
		return err
	}
	return workflow.Output("descriptor-digest", digest)
}
func main() { pantavisor.ToolMain("oras", []string{"version"}, run) }
