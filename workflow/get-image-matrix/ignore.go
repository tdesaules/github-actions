package main

import (
	"strings"

	"github.com/moby/patternmatcher"
	"github.com/moby/patternmatcher/ignorefile"
	"github.com/tdesaules/github-actions/shared/workflow"
)

func isIgnorePath(path string) bool {
	return path == ".containerignore" || path == ".dockerignore"
}

func hasIgnorePath(paths []string) bool {
	for _, path := range paths {
		if isIgnorePath(path) {
			return true
		}
	}
	return false
}

type ignoreChange struct {
	before *patternmatcher.PatternMatcher
	files  []string
	paths  []string
}

// Compare the effective context against the release, not the ignore file bytes.
// Both trees are needed: rules may expose additions or hide deleted inputs.
func loadIgnoreChange(base string, changed []string) (*ignoreChange, error) {
	change := &ignoreChange{}
	for _, path := range changed {
		if isIgnorePath(path) {
			change.paths = append(change.paths, path)
		}
	}
	if len(change.paths) == 0 {
		return change, nil
	}
	old, err := workflow.Git("ls-tree", "-r", "--name-only", "-z", base)
	if err != nil {
		return nil, err
	}
	current, err := workflow.Git("ls-tree", "-r", "--name-only", "-z", "HEAD")
	if err != nil {
		return nil, err
	}
	oldFiles := map[string]bool{}
	allFiles := map[string]bool{}
	for _, path := range strings.Split(old, "\x00") {
		oldFiles[path] = true
		allFiles[path] = true
	}
	for _, path := range strings.Split(current, "\x00") {
		allFiles[path] = true
	}
	for path := range allFiles {
		if path != "" {
			change.files = append(change.files, path)
		}
	}
	for _, name := range []string{".containerignore", ".dockerignore"} {
		if !oldFiles[name] {
			continue
		}
		data, err := workflow.Git("show", base+":"+name)
		if err != nil {
			return nil, err
		}
		patterns, err := ignorefile.ReadAll(strings.NewReader(data))
		if err != nil {
			return nil, err
		}
		change.before, err = patternmatcher.New(patterns)
		if err != nil {
			return nil, err
		}
		break
	}
	return change, nil
}

func (change *ignoreChange) affects(current dependencies) (bool, error) {
	if change == nil {
		return true, nil // No trustworthy baseline: retain conservative rebuilding.
	}
	if len(change.paths) == 0 {
		return false, nil // The current diff reverted an ignore rule to the release.
	}
	if current.uncertain {
		return true, nil
	}
	before := current
	before.ignore = change.before
	// A recipe may copy the ignore file itself (including COPY .). Its changed
	// bytes are an input even when the rules select exactly the same file set.
	for _, path := range change.paths {
		for _, deps := range []dependencies{before, current} {
			included, err := deps.affected(path)
			if err != nil || included {
				return included, err
			}
		}
	}
	for _, path := range change.files {
		old, err := before.affected(path)
		if err != nil {
			return false, err
		}
		now, err := current.affected(path)
		if err != nil || old != now {
			return old != now, err
		}
	}
	return false, nil
}
