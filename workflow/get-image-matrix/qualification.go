package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"

	"github.com/tdesaules/github-actions/shared/workflow"
	"go.yaml.in/yaml/v3"
)

type qualificationWorkflow struct {
	Jobs        map[string]map[string]any `yaml:"jobs"`
	Env         any                       `yaml:"env"`
	Defaults    any                       `yaml:"defaults"`
	Permissions any                       `yaml:"permissions"`
}

func readQualification(data []byte) (qualificationWorkflow, error) {
	var w qualificationWorkflow
	err := yaml.Unmarshal(data, &w)
	return w, err
}

// Only recognize conditions whose image scope is unambiguous. Unknown expressions
// stay applicable to every image rather than silently dropping required tests.
func appliesToImage(step map[string]any, image string) bool {
	condition, _ := step["if"].(string)
	condition = strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(strings.TrimSpace(condition), "${{"), "}}"))
	if match := regexp.MustCompile(`^matrix\.image == '([^']+)'$`).FindStringSubmatch(condition); match != nil {
		return image == match[1]
	}
	if match := regexp.MustCompile(`^contains\(fromJSON\('([^']+)'\), matrix\.image\)$`).FindStringSubmatch(condition); match != nil {
		var images []string
		if json.Unmarshal([]byte(match[1]), &images) == nil {
			for _, name := range images {
				if name == image {
					return true
				}
			}
			return false
		}
	}
	return true
}

func qualificationSteps(w qualificationWorkflow, image string) []map[string]any {
	result := []map[string]any{}
	steps, _ := w.Jobs["test"]["steps"].([]any)
	for _, raw := range steps {
		step, ok := raw.(map[string]any)
		if ok && appliesToImage(step, image) {
			result = append(result, step)
		}
	}
	return result
}

func workflowAffects(before, after qualificationWorkflow, image string) bool {
	if !reflect.DeepEqual(before.Env, after.Env) || !reflect.DeepEqual(before.Defaults, after.Defaults) || !reflect.DeepEqual(before.Permissions, after.Permissions) || !reflect.DeepEqual(before.Jobs["build"], after.Jobs["build"]) {
		return true
	}
	context := func(w qualificationWorkflow) map[string]any {
		result := map[string]any{}
		for key, value := range w.Jobs["test"] {
			if key != "steps" {
				result[key] = value
			}
		}
		return result
	}
	return !reflect.DeepEqual(context(before), context(after)) || !reflect.DeepEqual(qualificationSteps(before, image), qualificationSteps(after, image))
}

var fixtureReference = regexp.MustCompile(`tests/fixtures/[A-Za-z0-9_./-]+`)

func referencedFixture(step map[string]any, path string) bool {
	data, _ := json.Marshal(step)
	for _, reference := range fixtureReference.FindAllString(string(data), -1) {
		reference = strings.TrimSuffix(reference, "/")
		if path == reference || strings.HasPrefix(path, reference+"/") {
			return true
		}
	}
	return false
}

// Discover test consumers from the workflow itself, not a second action catalogue.
func qualificationChanges(components []component, paths []string, before string) (map[string]bool, error) {
	selected := map[string]bool{}
	currentData, err := os.ReadFile(".github/workflows/main.yml")
	if os.IsNotExist(err) {
		// Synthetic/external catalogues without an orchestration graph keep a
		// conservative fallback for workflow and fixture changes only.
		for _, p := range paths {
			if sourcePath(p) && (strings.HasPrefix(p, ".github/workflows/") || strings.HasPrefix(p, "tests/fixtures/")) {
				for _, c := range components {
					selected[c.image] = true
				}
			}
		}
		return selected, nil
	}
	if err != nil {
		return nil, err
	}
	current, err := readQualification(currentData)
	if err != nil {
		return nil, err
	}
	var previous qualificationWorkflow
	workflowChanged, previousKnown := false, false
	for _, p := range paths {
		workflowChanged = workflowChanged || p == ".github/workflows/main.yml"
	}
	if workflowChanged && regexp.MustCompile(`^[a-f0-9]{40}$`).MatchString(before) {
		data, err := workflow.Git("show", before+":.github/workflows/main.yml")
		if err == nil {
			previous, err = readQualification([]byte(data))
			if err != nil {
				fmt.Fprintln(os.Stderr, "Previous workflow cannot be parsed; selecting all qualification jobs")
			} else {
				previousKnown = true
			}
		}
	}
	helperDependencies := map[string]dependencies{}
	for _, c := range components {
		if workflowChanged && (!previousKnown || workflowAffects(previous, current, c.image)) {
			selected[c.image] = true
		}
		steps := qualificationSteps(current, c.image)
		if previousKnown {
			steps = append(steps, qualificationSteps(previous, c.image)...)
		}
		for _, step := range steps {
			id, _ := step["id"].(string)
			if strings.HasPrefix(id, "security-") && step["continue-on-error"] == true {
				continue // Advisory scans are not native qualification consumers.
			}
			uses, _ := step["uses"].(string)
			// Candidate steps are tested products, not shared qualification tools.
			if strings.HasPrefix(uses, "./.candidate-actions/") {
				uses = ""
			}
			directory := strings.TrimPrefix(uses, "./")
			var deps dependencies
			local := strings.HasPrefix(uses, "./")
			if local {
				var ok bool
				deps, ok = helperDependencies[directory]
				if !ok {
					if _, err := os.Stat(filepath.Join(directory, "Containerfile")); os.IsNotExist(err) {
						local = false
					} else {
						deps, err = buildDependencies(directory)
						if err != nil {
							return nil, err
						}
						helperDependencies[directory] = deps
					}
				}
			}
			for _, p := range paths {
				if strings.HasPrefix(p, "tests/fixtures/") && referencedFixture(step, p) {
					selected[c.image] = true
				}
				if !sourcePath(p) && filepath.Base(p) != "action.yml" {
					continue
				}
				if strings.HasPrefix(uses, "./") {
					if strings.HasPrefix(p, directory+"/") {
						selected[c.image] = true
						continue
					}
				}
				if local {
					if isIgnorePath(p) {
						continue
					} // Product/context selection handles these with a release baseline.
					affected, err := deps.affected(p)
					if err != nil {
						return nil, err
					}
					if affected {
						selected[c.image] = true
					}
				}
			}
		}
	}
	return selected, nil
}
