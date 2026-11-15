package main

import (
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
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

// Recognize only literal image/native-architecture comparisons and image lists,
// combined with parentheses, && and ||. Everything else remains conservative.
func scopedCondition(condition, image, arch string) (bool, bool) {
	lists := regexp.MustCompile(`contains\(fromJSON\('([^']+)'\), matrix\.image\)`)
	condition = lists.ReplaceAllStringFunc(condition, func(value string) string {
		var images []string
		if json.Unmarshal([]byte(lists.FindStringSubmatch(value)[1]), &images) != nil {
			return value
		}
		for _, name := range images {
			if name == image {
				return "true"
			}
		}
		return "false"
	})
	comparison := regexp.MustCompile(`matrix\.(image|arch)\s*==\s*'([^']+)'`)
	condition = comparison.ReplaceAllStringFunc(condition, func(value string) string {
		parts := comparison.FindStringSubmatch(value)
		actual := image
		if parts[1] == "arch" {
			if parts[2] != "amd64" && parts[2] != "arm64" {
				return value
			}
			actual = arch
		}
		return fmt.Sprint(actual == parts[2])
	})
	expression, err := parser.ParseExpr(condition)
	if err != nil {
		return false, false
	}
	var evaluate func(ast.Expr) (bool, bool)
	evaluate = func(e ast.Expr) (bool, bool) {
		switch v := e.(type) {
		case *ast.ParenExpr:
			return evaluate(v.X)
		case *ast.Ident:
			return v.Name == "true", v.Name == "true" || v.Name == "false"
		case *ast.BinaryExpr:
			left, leftKnown := evaluate(v.X)
			right, rightKnown := evaluate(v.Y)
			if !leftKnown || !rightKnown {
				return false, false
			}
			switch v.Op {
			case token.LAND:
				return left && right, true
			case token.LOR:
				return left || right, true
			}
		}
		return false, false
	}
	return evaluate(expression)
}

func appliesToImage(step map[string]any, image string) bool {
	condition, _ := step["if"].(string)
	condition = strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(strings.TrimSpace(condition), "${{"), "}}"))
	for _, arch := range []string{"amd64", "arm64"} {
		applies, known := scopedCondition(condition, image, arch)
		if !known || applies {
			return true
		}
	}
	return false
}

// The qualified builder constructs test-only harnesses from literal recipes.
// Generated candidate uses alone do not reveal those harness build dependencies.
// Scope this inference to the existing repository-root build contract.
func qualificationHarness(step map[string]any) string {
	if step["uses"] != "./oci/build-image" {
		return ""
	}
	inputs, _ := step["with"].(map[string]any)
	context, _ := inputs["context"].(string)
	if context != "" && context != "." {
		return ""
	}
	recipe, _ := inputs["containerfile"].(string)
	if recipe == "" || strings.Contains(recipe, "${{") || filepath.IsAbs(recipe) {
		return ""
	}
	recipe = filepath.ToSlash(filepath.Clean(recipe))
	if strings.HasPrefix(recipe, "../") || filepath.Base(recipe) != "Containerfile" {
		return ""
	}
	directory := filepath.Dir(recipe)
	if directory == "." {
		return ""
	}
	return directory
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
	workflowPath := ".github/workflows/image.yml"
	if _, err := os.Stat(workflowPath); os.IsNotExist(err) {
		workflowPath = ".github/workflows/main.yml"
	}
	currentData, err := os.ReadFile(workflowPath)
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
		workflowChanged = workflowChanged || p == workflowPath
	}
	if workflowChanged && regexp.MustCompile(`^[a-f0-9]{40}$`).MatchString(before) {
		data, err := workflow.Git("show", before+":"+workflowPath)
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
			harness := qualificationHarness(step)
			var harnessDeps dependencies
			harnessExists := false
			if harness != "" {
				harnessDeps, harnessExists = helperDependencies[harness]
				if !harnessExists {
					if _, err := os.Stat(filepath.Join(harness, "Containerfile")); err == nil {
						harnessDeps, err = buildDependencies(harness)
						if err != nil {
							return nil, err
						}
						helperDependencies[harness] = harnessDeps
						harnessExists = true
					} else if !os.IsNotExist(err) {
						return nil, err
					}
				}
			}
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
				if harness != "" {
					// Includes metadata and deletions even when the recipe only
					// copies main.go or no longer exists in the current checkout.
					if strings.HasPrefix(p, harness+"/") {
						selected[c.image] = true
					}
					if harnessExists && !isIgnorePath(p) {
						affected, err := harnessDeps.affected(p)
						if err != nil {
							return nil, err
						}
						if affected {
							selected[c.image] = true
						}
					}
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
