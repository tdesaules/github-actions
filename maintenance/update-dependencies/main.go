package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
)

func execute(args []string) error {
	flags := flag.NewFlagSet("update-dependencies", flag.ContinueOnError)
	repository := flags.String("repository", os.Getenv("INPUT_REPOSITORY"), "GitHub owner/repository")
	dryRun := flags.Bool("dry-run", os.Getenv("INPUT_DRY-RUN") == "true", "resolve updates without writing branches or PRs")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("unexpected positional arguments")
	}
	if !regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$`).MatchString(*repository) {
		return errors.New("repository must be owner/name")
	}
	if os.Getenv("RENOVATE_TOKEN") == "" {
		return errors.New("RENOVATE_TOKEN is required")
	}
	// Keep the repository explicit and disable autodiscovery. No Docker socket or
	// host package manager is used; Go is included for module artifact updates.
	env := os.Environ()
	filtered := env[:0]
	for _, value := range env {
		if !strings.HasPrefix(value, "RENOVATE_DRY_RUN=") {
			filtered = append(filtered, value)
		}
	}
	env = filtered
	if *dryRun {
		env = append(env, "RENOVATE_DRY_RUN=full")
	}
	for _, value := range []string{"RENOVATE_PLATFORM=github", "RENOVATE_AUTODISCOVER=false", "RENOVATE_BINARY_SOURCE=global", "RENOVATE_BASE_DIR=/tmp/renovate", "RENOVATE_ONBOARDING=false", "RENOVATE_REQUIRE_CONFIG=required"} {
		key, _, _ := strings.Cut(value, "=")
		filtered := env[:0]
		for _, old := range env {
			if !strings.HasPrefix(old, key+"=") {
				filtered = append(filtered, old)
			}
		}
		env = append(filtered, value)
	}
	command := exec.Command("/usr/local/renovate/node", "--use-openssl-ca", "/usr/local/renovate/dist/renovate.js", *repository)
	command.Env, command.Stdout, command.Stderr = env, os.Stdout, os.Stderr
	return command.Run()
}

func main() {
	if len(os.Args) == 2 && os.Args[1] == "--tool-version" {
		fmt.Println("Renovate 44.167.1 (braces depth guard)")
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
