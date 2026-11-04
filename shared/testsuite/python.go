package testsuite

import (
	"fmt"
	"path/filepath"
	"strings"
)

func pythonArgs(directory string) ([]string, error) {
	if directory == "" || strings.ContainsAny(directory, "\x00\r\n") || !filepath.IsLocal(directory) {
		return nil, fmt.Errorf("tests-directory must be a nonempty relative path inside the project")
	}
	root, err := filepath.Abs(".")
	if err != nil {
		return nil, err
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return nil, err
	}
	target, err := filepath.EvalSymlinks(filepath.Join(root, directory))
	if err != nil {
		return nil, err
	}
	relative, err := filepath.Rel(root, target)
	if err != nil || !filepath.IsLocal(relative) {
		return nil, fmt.Errorf("tests-directory must remain inside the project after resolving symlinks")
	}
	return []string{"-B", "-m", "unittest", "discover", "-s", directory, "-v"}, nil
}
