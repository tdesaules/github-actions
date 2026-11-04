package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/tdesaules/github-actions/shared/component"
	"github.com/tdesaules/github-actions/shared/workflow"
)

func run() error {
	directory := flag.String("directory", os.Getenv("INPUT_DIRECTORY"), "schema-2 component directory")
	flag.Parse()
	if flag.NArg() != 0 || *directory == "" {
		return fmt.Errorf("directory is required; positional arguments are not supported")
	}
	metadata, digest, err := component.Verify(*directory)
	if err != nil {
		return err
	}
	for _, output := range []struct{ name, value string }{
		{"digest", digest}, {"name", metadata.Name}, {"version", metadata.Version}, {"kind", metadata.Kind},
	} {
		if err := workflow.Output(output.name, output.value); err != nil {
			return err
		}
	}
	return nil
}

func main() { workflow.Main("test-component schema-2", run) }
