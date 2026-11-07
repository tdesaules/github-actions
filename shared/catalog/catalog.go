// Package catalog discovers image owners from the action source tree.
package catalog

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

var ImageLine = regexp.MustCompile(`(?m)^(?:  image|        BUILDER_IMAGE): docker://([^\s]+)$`)
var Pinned = regexp.MustCompile(`^.+@sha256:[a-f0-9]{64}$`)

// Reference permits only an action's own Containerfile as a source bootstrap.
// An empty reference is deliberately not a pullable or publishable image tag.
func Reference(data []byte, directory string) (name, reference string, err error) {
	if match := ImageLine.FindSubmatch(data); match != nil {
		reference = string(match[1])
		name = strings.Split(strings.Split(filepath.Base(reference), ":")[0], "@")[0]
		return name, reference, nil
	}
	if !regexp.MustCompile(`(?m)^  image: Containerfile$`).Match(data) || !regexp.MustCompile(`(?m)^  using: docker$`).Match(data) {
		return "", "", fmt.Errorf("%s: missing Docker image reference", directory)
	}
	name = filepath.Base(filepath.Dir(directory)) + "-" + filepath.Base(directory)
	info, err := os.Lstat(filepath.Join(directory, "Containerfile"))
	if err != nil {
		return "", "", err
	}
	if !info.Mode().IsRegular() {
		return "", "", fmt.Errorf("%s: source recipe must be an owned regular file", directory)
	}
	recipe, err := os.ReadFile(filepath.Join(directory, "Containerfile"))
	if err != nil {
		return "", "", err
	}
	final := strings.LastIndex(string(recipe), "FROM ")
	if final < 0 || !regexp.MustCompile(`(?m)^FROM [^\n]+ AS final$`).Match(recipe[final:]) || !regexp.MustCompile(`(?m)^LABEL [^\n]*\borg\.opencontainers\.image\.title="`+regexp.QuoteMeta(name)+`"(?:\s|$)`).Match(recipe[final:]) {
		return "", "", fmt.Errorf("%s: source image requires matching final-stage title", directory)
	}
	return name, "", nil
}

type Image struct{ Name, Directory, Reference string }

func Discover() ([]Image, error) {
	images := []Image{}
	owners := map[string]bool{}
	for _, family := range []string{"oci", "check", "lint", "workflow", "maintenance"} {
		paths, err := filepath.Glob(family + "/*/action.yml")
		if err != nil {
			return nil, err
		}
		for _, path := range paths {
			directory := filepath.Dir(path)
			if _, err := os.Stat(directory + "/Containerfile"); err != nil {
				if family == "workflow" && os.IsNotExist(err) {
					continue
				}
				return nil, err
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return nil, err
			}
			name, ref, err := Reference(data, directory)
			if err != nil {
				return nil, err
			}
			if family != "lint" && name != family+"-"+filepath.Base(directory) {
				return nil, fmt.Errorf("%s: wrong image owner", path)
			}
			if owners[name] {
				return nil, fmt.Errorf("duplicate owner for %s", name)
			}
			owners[name] = true
			images = append(images, Image{name, directory, ref})
		}
	}
	if len(images) == 0 {
		return nil, fmt.Errorf("empty catalogue")
	}
	sort.Slice(images, func(i, j int) bool { return images[i].Name < images[j].Name })
	return images, nil
}

func Read(path string, value any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, value)
}
func Write(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	if err = os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".state-")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	// Journals and review evidence are public build artifacts, not credentials.
	// Docker actions may run as root while the artifact uploader runs as the runner.
	if err = f.Chmod(0644); err != nil {
		f.Close()
		return err
	}
	if _, err = f.Write(append(data, '\n')); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}
func Run(binary string, args ...string) error {
	cmd := exec.Command(binary, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
