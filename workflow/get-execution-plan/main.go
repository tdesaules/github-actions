package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/tdesaules/github-actions/shared/plan"
)

func git(root string, args ...string) (string, error) {
	command := exec.Command("git", append([]string{"-c", "safe.directory=" + root, "-C", root}, args...)...)
	data, err := command.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git %s: %s: %w", strings.Join(args, " "), data, err)
	}
	return strings.TrimSuffix(string(data), "\n"), nil
}

func execute(args []string) error {
	flags := flag.NewFlagSet("get-execution-plan", flag.ContinueOnError)
	before := flags.String("before", os.Getenv("INPUT_BEFORE"), "base commit; defaults to the push event's before SHA")
	after := flags.String("after", os.Getenv("INPUT_AFTER"), "target commit; defaults to GITHUB_SHA or HEAD")
	full := flags.Bool("full", os.Getenv("INPUT_FULL") == "true", "select all components and tracked source files")
	root := flags.String("repository", ".", "repository root")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("unexpected positional arguments")
	}
	absolute, err := filepath.Abs(*root)
	if err != nil {
		return err
	}
	if *after == "" {
		*after = os.Getenv("GITHUB_SHA")
	}
	if *after == "" {
		*after = "HEAD"
	}
	commit, err := git(absolute, "rev-parse", "--verify", "--end-of-options", *after+"^{commit}")
	if err != nil {
		return err
	}
	head, err := git(absolute, "rev-parse", "HEAD")
	if err != nil {
		return err
	}
	if head != commit {
		return errors.New("repository checkout must match the target commit")
	}
	if *before == "" && os.Getenv("GITHUB_EVENT_NAME") == "push" && !strings.HasPrefix(os.Getenv("GITHUB_REF"), "refs/tags/") {
		data, err := os.ReadFile(os.Getenv("GITHUB_EVENT_PATH"))
		if err != nil {
			return err
		}
		var event struct {
			Before string `json:"before"`
		}
		if err = json.Unmarshal(data, &event); err != nil {
			return err
		}
		*before = event.Before
	}
	if strings.Trim(*before, "0") == "" {
		base, err := git(absolute, "merge-base", commit, "origin/main")
		if err != nil || base == commit {
			*full = true
		} else {
			*before = base
		}
	}
	var paths []string
	if !*full {
		base, err := git(absolute, "rev-parse", "--verify", "--end-of-options", *before+"^{commit}")
		if err != nil {
			return err
		}
		changed, err := git(absolute, "diff", "--name-only", "--no-renames", "-z", base, commit, "--")
		if err != nil {
			return err
		}
		for _, path := range strings.Split(changed, "\x00") {
			if path != "" {
				paths = append(paths, path)
			}
		}
	}
	result, err := plan.Select(absolute, paths, *full)
	if err != nil {
		return err
	}
	data, err := json.Marshal(result)
	if err != nil {
		return err
	}
	fmt.Println(string(data))
	if output := os.Getenv("GITHUB_OUTPUT"); output != "" {
		file, err := os.OpenFile(output, os.O_APPEND|os.O_WRONLY, 0600)
		if err != nil {
			return err
		}
		defer file.Close()
		values := map[string]any{"plan": result, "matrix": result.Matrix, "checks": result.Checks, "actions": result.Actions,
			"has-work": len(result.Matrix.Include) > 0, "go-packages": result.GoPackages, "orchestration-tests": result.OrchestrationTests}
		for _, kind := range []string{"github-workflow", "containerfile", "shell", "yaml", "toml", "markdown", "links", "action-metadata"} {
			files := result.Checks[kind]
			if files == nil {
				files = []string{}
			}
			values[kind+"-files"], values["has-"+kind] = files, len(files) > 0
		}
		for name, value := range values {
			encoded, err := json.Marshal(value)
			if err != nil {
				return err
			}
			if _, err = fmt.Fprintf(file, "%s=%s\n", name, encoded); err != nil {
				return err
			}
		}
	}
	return nil
}

func main() {
	if len(os.Args) == 2 && os.Args[1] == "--tool-version" {
		fmt.Println("get-execution-plan 1")
		return
	}
	if err := execute(os.Args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return
		}
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
