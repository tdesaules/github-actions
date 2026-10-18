package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"go.yaml.in/yaml/v3"
)

func execute(args []string) error {
	flags := flag.NewFlagSet("new-candidate-action", flag.ContinueOnError)
	directory := flags.String("directory", os.Getenv("INPUT_DIRECTORY"), "source action directory")
	image := flags.String("image", os.Getenv("INPUT_IMAGE"), "loaded localhost tag or registry candidate digest")
	destination := flags.String("destination", os.Getenv("INPUT_DESTINATION"), "generated candidate directory")
	if err := flags.Parse(args); err != nil {
		return err
	}
	local := regexp.MustCompile(`^localhost/[a-z0-9/_.-]+:[a-zA-Z0-9_.-]+$`).MatchString(*image)
	pinned := regexp.MustCompile(`^[a-z0-9][a-z0-9.-]*(?::[0-9]+)?/[a-z0-9][a-z0-9/_.-]*@sha256:[a-f0-9]{64}$`).MatchString(*image)
	if flags.NArg() != 0 || *directory == "" || *destination == "" || (!local && !pinned) {
		return errors.New("directory, destination and a localhost tag or registry digest are required")
	}
	source, err := filepath.Abs(*directory)
	if err != nil {
		return err
	}
	target, err := filepath.Abs(*destination)
	if err != nil {
		return err
	}
	if source == target || strings.HasPrefix(source, target+string(filepath.Separator)) || strings.HasPrefix(target, source+string(filepath.Separator)) {
		return errors.New("candidate directory must be separate from the source action")
	}
	data, err := os.ReadFile(filepath.Join(source, "action.yml"))
	if err != nil {
		return err
	}
	var metadata map[string]any
	if err = yaml.Unmarshal(data, &metadata); err != nil {
		return err
	}
	runs, ok := metadata["runs"].(map[string]any)
	if !ok {
		return errors.New("missing action runs mapping")
	}
	switch runs["using"] {
	case "docker":
		runs["image"] = "Dockerfile"
		if pinned {
			runs["image"] = "docker://" + *image
		}
	case "composite":
		steps, ok := runs["steps"].([]any)
		if !ok {
			return errors.New("missing composite steps")
		}
		found := false
		for _, item := range steps {
			step, ok := item.(map[string]any)
			if !ok {
				continue
			}
			env, ok := step["env"].(map[string]any)
			if !ok {
				continue
			}
			if _, ok = env["BUILDER_IMAGE"]; ok {
				env["BUILDER_IMAGE"] = "docker://" + *image
				found = true
			}
		}
		if !found {
			return errors.New("only the Buildah launcher has a candidate image override")
		}
	default:
		return errors.New("unsupported action runtime")
	}
	data, err = yaml.Marshal(metadata)
	if err != nil {
		return err
	}
	if err = os.MkdirAll(target, 0755); err != nil {
		return err
	}
	if err = os.WriteFile(filepath.Join(target, "action.yml"), data, 0644); err != nil {
		return err
	}
	if local {
		return os.WriteFile(filepath.Join(target, "Dockerfile"), []byte("FROM "+*image+"\n"), 0644)
	}
	return nil
}

func main() {
	if len(os.Args) == 2 && os.Args[1] == "--tool-version" {
		fmt.Println("new-candidate-action 1")
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
