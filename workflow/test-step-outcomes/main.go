package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"github.com/tdesaules/github-actions/shared/workflow"
	"os"
)

func run() error {
	outcomes := flag.String("outcomes", os.Getenv("INPUT_OUTCOMES"), "GitHub steps JSON")
	expected := flag.String("expected-failures", workflow.Default("INPUT_EXPECTED-FAILURES", "[]"), "IDs that must have failed")
	absent := flag.String("absent-files", workflow.Default("INPUT_ABSENT-FILES", "[]"), "outputs that must not exist")
	flag.Parse()
	if flag.NArg() != 0 {
		return fmt.Errorf("unexpected arguments")
	}
	var steps map[string]struct {
		Outcome string `json:"outcome"`
	}
	var names, paths []string
	if err := json.Unmarshal([]byte(*outcomes), &steps); err != nil || steps == nil {
		return fmt.Errorf("outcomes must be a JSON object")
	}
	if err := json.Unmarshal([]byte(*expected), &names); err != nil || names == nil {
		return fmt.Errorf("expected-failures must be a JSON array")
	}
	if err := json.Unmarshal([]byte(*absent), &paths); err != nil || paths == nil {
		return fmt.Errorf("absent-files must be a JSON array")
	}
	for _, name := range names {
		step, ok := steps[name]
		if !ok || step.Outcome != "failure" {
			return fmt.Errorf("%s: expected failure, got %q", name, step.Outcome)
		}
	}
	for _, path := range paths {
		if _, err := os.Lstat(path); !os.IsNotExist(err) {
			return fmt.Errorf("failed operation left an output or inaccessible path: %s", path)
		}
	}
	return workflow.Output("verified", true)
}
func main() { workflow.Main("test-step-outcomes 1", run) }
