package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/tdesaules/github-actions/shared/workflow"
)

var repositoryPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9.-]*(/[a-z0-9][a-z0-9._-]*)+$`)

func localPath(value string) error {
	if value == "" || strings.HasPrefix(value, "-") || filepath.IsAbs(value) || filepath.Clean(value) == ".." ||
		strings.HasPrefix(filepath.Clean(value), ".."+string(filepath.Separator)) {
		return fmt.Errorf("expected a workspace-relative path: %q", value)
	}
	return nil
}

func validate(recipe, output, repository, platform string) error {
	if err := localPath(recipe); err != nil {
		return err
	}
	if err := localPath(output); err != nil {
		return err
	}
	if filepath.Clean(recipe) == filepath.Clean(output) {
		return fmt.Errorf("output must not overwrite recipe")
	}
	if !repositoryPattern.MatchString(repository) || len(strings.Split(repository, "/")) < 3 {
		return fmt.Errorf("repository must be a lowercase registry/namespace/image without tag")
	}
	if platform != "linux/amd64" && platform != "linux/arm64" {
		return fmt.Errorf("unsupported platform %q", platform)
	}
	return nil
}

func run() error {
	directory := flag.String("directory", ".", "BlueBuild project directory")
	recipe := flag.String("recipe", "recipes/recipe.yml", "BlueBuild recipe")
	output := flag.String("output", ".build/Containerfile", "generated Containerfile")
	repository := flag.String("repository", "", "publication repository without tag")
	platform := flag.String("platform", "", "native target platform")
	flag.Parse()
	if flag.NArg() != 0 {
		return fmt.Errorf("unexpected positional arguments")
	}
	if err := validate(*recipe, *output, *repository, *platform); err != nil {
		return err
	}
	if err := localPath(*directory); err != nil {
		return err
	}
	if err := os.Chdir(*directory); err != nil {
		return err
	}
	// Buildah consumes these generated files from the same workspace. BlueBuild
	// validates the recipe and resolves the selected platform's current base.
	if err := os.MkdirAll(filepath.Dir(*output), 0755); err != nil {
		return err
	}
	parts := strings.Split(*repository, "/")
	cmd := exec.Command("bluebuild", "generate", *recipe,
		"--registry", parts[0],
		"--registry-namespace", strings.Join(parts[1:len(parts)-1], "/"),
		"--platform", *platform, "--inspect-driver", "oci-client",
		"--output", *output)
	// Generation only probes Podman's client version; it does not start an
	// engine. Reuse the action's existing namespace instead of attempting a
	// nested user-namespace re-exec, which Docker's default seccomp rejects.
	cmd.Env = append(os.Environ(), "_CONTAINERS_USERNS_CONFIGURED=1")
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("render BlueBuild recipe: %w", err)
	}
	info, err := os.Stat(*output)
	if err != nil {
		return err
	}
	if info.Size() == 0 {
		return fmt.Errorf("empty generated Containerfile")
	}
	return workflow.Output("containerfile", filepath.Join(*directory, *output))
}

func main() { workflow.Main("render-bluebuild 1", run) }
