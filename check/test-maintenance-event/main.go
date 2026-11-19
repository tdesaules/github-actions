package main

import (
	"fmt"
	"github.com/tdesaules/github-actions/shared/workflow"
	"os"
)

func valid(ref, event string) bool {
	return ref == "refs/heads/main" && (event == "schedule" || event == "workflow_dispatch")
}

func run() error {
	if len(os.Args) != 1 || !valid(os.Getenv("GITHUB_REF"), os.Getenv("GITHUB_EVENT_NAME")) {
		return fmt.Errorf("maintenance requires schedule or dispatch on main")
	}
	return nil
}
func main() { workflow.Main("check-test-maintenance-event 1", run) }
