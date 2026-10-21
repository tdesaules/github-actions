package publication

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/tdesaules/github-actions/shared/catalog"
	action "github.com/tdesaules/github-actions/shared/runtime"
)

var matrixBinary = "/matrix"

func Prepare(directory, owner, planPath string) error {
	if !regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`).MatchString(owner) {
		return fmt.Errorf("invalid lowercase owner")
	}
	revision, err := Revision()
	if err != nil {
		return err
	}
	version := "0.0.0-bootstrap"
	if planPath != "" {
		var plan ReleasePlan
		if err = catalog.Read(planPath, &plan); err != nil {
			return err
		}
		if plan.Base != revision || !Stable.MatchString(plan.Version) {
			return fmt.Errorf("invalid or stale release plan")
		}
		version = plan.Version
	}
	cmd := exec.Command(matrixBinary, "--event=workflow_dispatch", "--ref=refs/heads/main", "--release-plan="+planPath, "--reuse-artifacts=false")
	cmd.Env = append(os.Environ(), "GITHUB_OUTPUT=")
	cmd.Stderr = os.Stderr
	data, err := cmd.Output()
	if err != nil {
		return err
	}
	var matrix struct {
		Include []struct {
			Image, Arch string
			Rebuild     bool
		} `json:"include"`
	}
	found := false
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "matrix=") {
			if err = json.Unmarshal([]byte(strings.TrimPrefix(line, "matrix=")), &matrix); err != nil {
				return err
			}
			found = true
		}
	}
	if !found {
		return fmt.Errorf("image matrix output missing")
	}
	expected := map[string]bool{}
	for _, job := range matrix.Include {
		if job.Rebuild {
			expected[job.Image+"/"+job.Arch] = true
		}
	}
	type metadata struct {
		Image    string `json:"image"`
		Arch     string `json:"arch"`
		Version  string `json:"version"`
		Revision string `json:"revision"`
		SHA256   string `json:"archive_sha256"`
		Tested   bool   `json:"tested"`
	}
	archives := map[string]map[string]Archive{}
	seen := map[string]bool{}
	candidateVersion := ""
	err = filepath.WalkDir(directory, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("symlink in publication artifacts: %s", path)
		}
		if entry.IsDir() || entry.Name() != "metadata.json" {
			return nil
		}
		var m metadata
		if err := catalog.Read(path, &m); err != nil {
			return err
		}
		key := m.Image + "/" + m.Arch
		if !expected[key] || seen[key] || !m.Tested || m.Revision != revision || m.Version == "" {
			return fmt.Errorf("unexpected, duplicate, untested or stale artifact: %s", path)
		}
		if candidateVersion != "" && candidateVersion != m.Version {
			return fmt.Errorf("mixed candidate versions")
		}
		candidateVersion = m.Version
		archive := filepath.Join(filepath.Dir(path), "image.oci.tar")
		sum, err := Checksum(archive)
		if err != nil {
			return err
		}
		if sum != m.SHA256 {
			return fmt.Errorf("archive checksum mismatch")
		}
		relative, err := filepath.Rel(directory, archive)
		if err != nil {
			return err
		}
		if archives[m.Image] == nil {
			archives[m.Image] = map[string]Archive{}
		}
		archives[m.Image][m.Arch] = Archive{relative, sum}
		seen[key] = true
		return nil
	})
	if err != nil {
		return err
	}
	if len(seen) != len(expected) {
		return fmt.Errorf("missing expected qualification artifacts: have %d, need %d", len(seen), len(expected))
	}
	images, err := catalog.Discover()
	if err != nil {
		return err
	}
	state := State{Version: version, Revision: revision, Images: map[string]*Image{}}
	for _, image := range images {
		item := &Image{Archives: archives[image.Name]}
		var raw []byte
		if item.Archives == nil {
			if !catalog.Pinned.MatchString(image.Reference) {
				return fmt.Errorf("unchanged image must be pinned: %s", image.Name)
			}
			item.Reference = image.Reference
			item.Reused = true
			item.Published = true
			raw, err = Raw(item.Reference)
			if err != nil {
				return err
			}
			if Hash(raw) != strings.Split(item.Reference, "@")[1] {
				return fmt.Errorf("reused digest mismatch")
			}
		} else {
			paths, err := ArchivePaths(directory, item)
			if err != nil {
				return err
			}
			layout, err := os.MkdirTemp("", "publication-layout-")
			if err != nil {
				return err
			}
			raw, err = action.AssembleIndex(paths, layout)
			os.RemoveAll(layout)
			if err != nil {
				return err
			}
			item.Reference = "ghcr.io/" + owner + "/github-actions/" + image.Name + "@" + Hash(raw)
		}
		item.Platforms, err = Children(raw)
		if err != nil {
			return err
		}
		for arch, a := range item.Archives {
			digest, err := ArchiveDigest(filepath.Join(directory, a.Path))
			if err != nil {
				return err
			}
			if item.Platforms[arch] != digest {
				return fmt.Errorf("archive architecture disagrees with qualification metadata")
			}
		}
		if err = os.MkdirAll(filepath.Join(directory, "indexes"), 0755); err != nil {
			return err
		}
		if err = os.WriteFile(filepath.Join(directory, "indexes", image.Name+".json"), raw, 0644); err != nil {
			return err
		}
		state.Images[image.Name] = item
	}
	return Save(directory, state)
}
