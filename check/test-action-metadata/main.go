package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/tdesaules/github-actions/shared/check"
	"go.yaml.in/yaml/v3"
)

type parameter struct {
	Description string  `yaml:"description"`
	Required    *bool   `yaml:"required"`
	Default     *string `yaml:"default"`
}

func validate(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var metadata struct {
		Name        string               `yaml:"name"`
		Description string               `yaml:"description"`
		Inputs      map[string]parameter `yaml:"inputs"`
		Outputs     map[string]parameter `yaml:"outputs"`
		Runs        struct {
			Using string            `yaml:"using"`
			Image string            `yaml:"image"`
			Args  []string          `yaml:"args"`
			Env   map[string]string `yaml:"env"`
			Steps []struct {
				Uses string            `yaml:"uses"`
				Run  string            `yaml:"run"`
				Env  map[string]string `yaml:"env"`
			} `yaml:"steps"`
		} `yaml:"runs"`
	}
	if err := yaml.Unmarshal(data, &metadata); err != nil {
		return err
	}
	if metadata.Name == "" || metadata.Description == "" {
		return fmt.Errorf("name and description are required")
	}
	for _, parameters := range []map[string]parameter{metadata.Inputs, metadata.Outputs} {
		for name, parameter := range parameters {
			if !regexp.MustCompile(`^[a-z][a-z0-9-]*$`).MatchString(name) || parameter.Description == "" {
				return fmt.Errorf("invalid or undocumented parameter %q", name)
			}
		}
	}
	directory := filepath.Dir(path)
	builder := filepath.Base(directory) == "build-image" && filepath.Base(filepath.Dir(directory)) == "oci"
	switch metadata.Runs.Using {
	case "docker":
		if !strings.HasPrefix(metadata.Runs.Image, "docker://") {
			return fmt.Errorf("native action must reference a prebuilt Docker image")
		}
		forwarded := map[string]bool{}
		for _, value := range metadata.Runs.Args {
			forwarded[value] = true
		}
		for _, value := range metadata.Runs.Env {
			forwarded[value] = true
		}
		for name := range metadata.Inputs {
			expression := "${{ inputs." + name + " }}"
			if !forwarded["--"+name+"="+expression] && !forwarded[expression] {
				return fmt.Errorf("input %s must be forwarded explicitly", name)
			}
		}
	case "composite":
		if len(metadata.Runs.Steps) == 0 {
			return fmt.Errorf("composite must assemble operations")
		}
		if builder && (len(metadata.Runs.Steps) != 1 || !strings.HasPrefix(metadata.Runs.Steps[0].Env["BUILDER_IMAGE"], "docker://")) {
			return fmt.Errorf("Buildah requires its narrow launcher")
		}
		for _, step := range metadata.Runs.Steps {
			if !builder && (step.Run != "" || step.Uses == "") {
				return fmt.Errorf("orchestration composites must assemble actions with uses")
			}
		}
	default:
		return fmt.Errorf("unsupported action runtime %q", metadata.Runs.Using)
	}
	if metadata.Runs.Using == "docker" || builder {
		for _, name := range []string{"Containerfile", "main.go"} {
			info, err := os.Stat(filepath.Join(directory, name))
			if err != nil {
				return err
			}
			if !info.Mode().IsRegular() {
				return fmt.Errorf("action must own a regular %s", name)
			}
		}
	}
	return nil
}

func main() { check.Main("action-metadata 1", validate) }
