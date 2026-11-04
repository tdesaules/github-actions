package main

import (
	"github.com/tdesaules/github-actions/shared/component"
	"github.com/tdesaules/github-actions/shared/pantavisor"
	"github.com/tdesaules/github-actions/shared/workflow"
	"os"
)

func run() error {
	directory, registry, user := pantavisor.Input("directory", ""), pantavisor.Input("registry", ""), pantavisor.Input("registry-username", "")
	if err := pantavisor.Parse(); err != nil {
		return err
	}
	reference, descriptor, err := component.Publish(*directory, *registry, *user, os.Getenv("INPUT_REGISTRY-PASSWORD"))
	if err != nil {
		return err
	}
	if err := workflow.Output("reference", reference); err != nil {
		return err
	}
	return workflow.Output("descriptor-digest", descriptor)
}
func main() { pantavisor.ToolMain("oras", []string{"version"}, run) }
