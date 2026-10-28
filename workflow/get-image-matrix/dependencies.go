package main

import (
	"encoding/csv"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/moby/buildkit/frontend/dockerfile/parser"
	"github.com/moby/buildkit/frontend/dockerfile/shell"
	"github.com/moby/patternmatcher"
	"github.com/moby/patternmatcher/ignorefile"
)

type dependencies struct {
	sources   []string
	uncertain bool
	ignore    *patternmatcher.PatternMatcher
}

func buildDependencies(directory string) (dependencies, error) {
	var result dependencies
	for _, name := range []string{".containerignore", ".dockerignore"} {
		f, err := os.Open(name)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return result, err
		}
		patterns, err := ignorefile.ReadAll(f)
		f.Close()
		if err != nil {
			return result, err
		}
		result.ignore, err = patternmatcher.New(patterns)
		if err != nil {
			return result, err
		}
		break
	}
	f, err := os.Open(filepath.Join(directory, "Containerfile"))
	if err != nil {
		return result, err
	}
	defer f.Close()
	recipe, err := parser.Parse(f)
	if err != nil {
		return result, err
	}
	lex := shell.NewLex(recipe.EscapeToken)
	lex.SkipUnsetEnv = true
	for _, node := range recipe.AST.Children {
		switch strings.ToLower(node.Value) {
		case "copy", "add":
			from := false
			for _, option := range node.Flags {
				if strings.HasPrefix(option, "--from=") || option == "--from" {
					from = true
				}
			}
			if from {
				continue
			} // A stage/image source is not a checkout dependency.
			args := []string{}
			for n := node.Next; n != nil; n = n.Next {
				args = append(args, n.Value)
			}
			if !node.Attributes["json"] {
				args, err = lex.ProcessWords(strings.Join(args, " "), shell.EnvsFromSlice(nil))
				if err != nil {
					return result, err
				}
			}
			if len(args) < 2 {
				return result, fmt.Errorf("%s: incomplete COPY/ADD", directory)
			}
			for _, source := range args[:len(args)-1] {
				if strings.HasPrefix(source, "<<") || strings.Contains(source, "://") || strings.HasPrefix(source, "git@") {
					continue
				}
				if strings.Contains(source, "$") {
					result.uncertain = true
					continue
				}
				result.sources = append(result.sources, filepath.ToSlash(filepath.Clean(strings.TrimLeft(source, "/"))))
			}
		case "run":
			for _, option := range node.Flags {
				if !strings.HasPrefix(option, "--mount=") {
					continue
				}
				fields, err := csv.NewReader(strings.NewReader(strings.TrimPrefix(option, "--mount="))).Read()
				if err != nil {
					result.uncertain = true
					continue
				}
				values := map[string]string{}
				for _, field := range fields {
					k, v, _ := strings.Cut(field, "=")
					values[k] = v
				}
				if values["from"] != "" || (values["type"] != "" && values["type"] != "bind") {
					continue
				}
				source := values["source"]
				if source == "" {
					source = values["src"]
				}
				if source == "" {
					source = "."
				}
				if strings.Contains(source, "$") {
					result.uncertain = true
				} else {
					result.sources = append(result.sources, filepath.ToSlash(filepath.Clean(strings.TrimLeft(source, "/"))))
				}
			}
		}
	}
	if result.uncertain {
		fmt.Fprintf(os.Stderr, "%s: variable build-context dependency; conservatively selecting context changes\n", directory)
	}
	return result, nil
}

func (d dependencies) affected(changed string) (bool, error) {
	changed = filepath.ToSlash(filepath.Clean(changed))
	if d.ignore != nil {
		ignored, err := d.ignore.MatchesOrParentMatches(changed)
		if err != nil {
			return false, err
		}
		if ignored {
			return false, nil
		}
	}
	if d.uncertain {
		return true, nil
	}
	for _, source := range d.sources {
		if source == "." || changed == source || strings.HasPrefix(changed, strings.TrimSuffix(source, "/")+"/") {
			return true, nil
		}
		matcher, err := patternmatcher.New([]string{source})
		if err != nil {
			return false, err
		}
		matched, err := matcher.MatchesOrParentMatches(changed)
		if err != nil {
			return false, err
		}
		if matched {
			return true, nil
		}
	}
	return false, nil
}
