// Package check runs one fixed, read-only check over explicitly selected files.
package check

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
)

type Result struct {
	File    string `json:"file"`
	Passed  bool   `json:"passed"`
	Details string `json:"details,omitempty"`
}

// External never invokes a shell or accepts executable arguments from inputs.
func External(binary string, args ...string) func(string) error {
	return func(path string) error {
		cmd := exec.Command(binary, append(append([]string{}, args...), path)...)
		data, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("%s: %w", strings.TrimSpace(string(data)), err)
		}
		return nil
	}
}

func files(value string) ([]string, error) {
	var paths []string
	if err := json.Unmarshal([]byte(value), &paths); err != nil || len(paths) == 0 {
		return nil, errors.New("files must be a nonempty JSON array of file paths; globs are not expanded")
	}
	seen := map[string]bool{}
	for i, path := range paths {
		if path == "" || strings.ContainsAny(path, "\r\n\x00") {
			return nil, errors.New("invalid file path")
		}
		absolute, err := filepath.Abs(path)
		if err != nil {
			return nil, err
		}
		info, err := os.Stat(absolute)
		if err != nil {
			return nil, err
		}
		if !info.Mode().IsRegular() {
			return nil, fmt.Errorf("not a regular file: %s", path)
		}
		if seen[absolute] {
			return nil, fmt.Errorf("duplicate file: %s", path)
		}
		seen[absolute] = true
		paths[i] = absolute // Leading dashes cannot become tool options.
	}
	return paths, nil
}

func run(args []string, validate func(string) error, options ...func(*flag.FlagSet)) error {
	flags := flag.NewFlagSet("check", flag.ContinueOnError)
	selected := flags.String("files", os.Getenv("INPUT_FILES"), "JSON array of files to check")
	for _, option := range options {
		option(flags)
	}
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("unexpected positional arguments")
	}
	paths, err := files(*selected)
	if err != nil {
		return err
	}
	results := make([]Result, 0, len(paths))
	failed := false
	for _, path := range paths {
		err := validate(path)
		result := Result{File: path, Passed: err == nil}
		if err != nil {
			result.Details = err.Error()
			failed = true
		}
		results = append(results, result)
	}
	// The report goes to stdout, so source mounts and the rootfs may be read-only.
	if err = json.NewEncoder(os.Stdout).Encode(results); err != nil {
		return err
	}
	if output := os.Getenv("GITHUB_OUTPUT"); output != "" {
		f, err := os.OpenFile(output, os.O_APPEND|os.O_WRONLY, 0600)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintf(f, "checked=%d\npassed=%t\n", len(results), !failed)
		closeErr := f.Close()
		if err != nil {
			return err
		}
		if closeErr != nil {
			return closeErr
		}
	}
	if failed {
		return errors.New("one or more files failed validation")
	}
	return nil
}

// Main binds validation at build time, not through an operation selector.
func Main(version string, validate func(string) error, options ...func(*flag.FlagSet)) {
	if len(os.Args) == 2 && os.Args[1] == "--tool-version" {
		fmt.Println(version)
		return
	}
	// Match the runner-owned output file before writing outputs or reading sources.
	if os.Getuid() == 0 && os.Getenv("GITHUB_OUTPUT") != "" {
		info, err := os.Stat(os.Getenv("GITHUB_OUTPUT"))
		if err == nil {
			stat := info.Sys().(*syscall.Stat_t)
			if stat.Uid != 0 {
				if err = syscall.Setgroups([]int{}); err == nil {
					err = syscall.Setgid(int(stat.Gid))
				}
				if err == nil {
					err = syscall.Setuid(int(stat.Uid))
				}
			}
		}
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}
	if err := run(os.Args[1:], validate, options...); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return
		}
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
