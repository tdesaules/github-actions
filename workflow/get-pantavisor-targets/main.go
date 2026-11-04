package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path"
	"strings"

	"github.com/tdesaules/github-actions/shared/workflow"
)

type selection struct {
	Check bool
	Build bool
	Test  bool
}

func selectTargets(project string, files []string, full bool) (selection, error) {
	if project != "core" && project != "k0s" && project != "system" {
		return selection{}, fmt.Errorf("project must be core, k0s or system")
	}
	result := selection{Check: full, Build: full, Test: full}
	for _, file := range files {
		if file == "" || path.IsAbs(file) || path.Clean(file) != file || file == "." || file == ".." || strings.HasPrefix(file, "../") || strings.ContainsAny(file, "\x00\r\n\\") {
			return selection{}, fmt.Errorf("invalid repository-relative changed path %q", file)
		}
		// Documentation alone never constructs a Yocto image. Actual file lists
		// for lint are emitted independently by get-changed-files.
		if strings.HasPrefix(file, "docs/") || strings.EqualFold(path.Ext(file), ".md") || strings.EqualFold(path.Ext(file), ".markdown") {
			continue
		}
		result.Check = true
		if strings.HasPrefix(file, "tests/") {
			continue
		}
		// Unknown inputs select conservatively, including deleted inputs. Source
		// provenance covers runtime/configuration as well as recipes: until a
		// reusable BSP/rootfs is supplied, assembly changes need a fresh build.
		result.Build, result.Test = true, true
	}
	return result, nil
}

func run() error {
	project := flag.String("project", os.Getenv("INPUT_PROJECT"), "core, k0s or system")
	filesJSON := flag.String("files", workflow.Default("INPUT_FILES", "[]"), "changed paths JSON array, including deleted paths")
	full := flag.Bool("full", os.Getenv("INPUT_FULL") == "true", "select the complete qualification set")
	flag.Parse()
	if flag.NArg() != 0 {
		return fmt.Errorf("unexpected positional arguments")
	}
	var files []string
	if err := json.Unmarshal([]byte(*filesJSON), &files); err != nil || files == nil {
		return fmt.Errorf("files must be a JSON array of strings")
	}
	selected, err := selectTargets(*project, files, *full)
	if err != nil {
		return err
	}
	for _, item := range []struct {
		name  string
		value bool
	}{
		{"check-required", selected.Check},
		{"build-required", selected.Build},
		{"test-required", selected.Test},
	} {
		if err := workflow.Output(item.name, item.value); err != nil {
			return err
		}
	}
	return nil
}

func main() { workflow.Main("get-pantavisor-targets", run) }
