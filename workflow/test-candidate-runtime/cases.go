package main

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/tdesaules/github-actions/shared/candidate"
)

func actionCase(root *candidate.Root, results *[]result, name, directory string, args, env []string, success bool, diagnostic string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	cmd, err := root.Command(ctx, "/entrypoint", args...)
	if err != nil {
		return err
	}
	cmd.Dir = directory
	cmd.Env = append(cmd.Env, env...)
	out, err := cmd.CombinedOutput()
	code := 0
	if err != nil {
		var exit *exec.ExitError
		if !errors.As(err, &exit) || ctx.Err() != nil {
			return fmt.Errorf("%s: %w: %s", name, err, out)
		}
		code = exit.ExitCode()
	}
	passed := (code == 0) == success && (diagnostic == "" || strings.Contains(string(out), diagnostic))
	*results = append(*results, result{name, passed, code, string(out)})
	if !passed {
		return fmt.Errorf("%s: exit=%d output=%s", name, code, out)
	}
	return nil
}
