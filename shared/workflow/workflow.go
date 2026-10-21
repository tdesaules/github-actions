// Package workflow supplies Git and output primitives to fixed-operation actions.
package workflow

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

func Git(args ...string) (string, error) {
	// A container UID can differ from the owner of the checkout mount.
	root, err := os.Getwd()
	if err != nil {
		return "", err
	}
	cmd := exec.Command("git", append([]string{"-c", "safe.directory=" + root}, args...)...)
	cmd.Stderr = os.Stderr
	data, err := cmd.Output()
	return string(data), err
}

func Paths(before, after string) ([]string, error) {
	data, err := Git("diff", "--name-only", "--no-renames", "-z", before, after, "--")
	return SplitPaths(data), err
}

func SplitPaths(data string) []string {
	if data == "" {
		return []string{}
	}
	return strings.Split(strings.TrimSuffix(data, "\x00"), "\x00")
}

func Output(name string, value any) error {
	text, ok := value.(string)
	if !ok {
		data, err := json.Marshal(value)
		if err != nil {
			return err
		}
		text = string(data)
	}
	if strings.ContainsAny(text, "\r\n") {
		return fmt.Errorf("multiline output %s", name)
	}
	line := name + "=" + text + "\n"
	fmt.Print(line)
	if path := os.Getenv("GITHUB_OUTPUT"); path != "" {
		f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0600)
		if err != nil {
			return err
		}
		_, err = f.WriteString(line)
		closeErr := f.Close()
		if err != nil {
			return err
		}
		return closeErr
	}
	return nil
}

func Main(version string, run func() error) {
	if len(os.Args) == 2 && os.Args[1] == "--tool-version" {
		fmt.Println(version)
		return
	}
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func Default(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}
