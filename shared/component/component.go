// Package component verifies portable Pantavisor schema-2 exports independently
// of any producer checkout. An OCI image manifest is not a component descriptor.
package component

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
)

var hashPattern = regexp.MustCompile(`^[a-f0-9]{64}$`)
var namePattern = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)
var versionPattern = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$`)

type File struct {
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
}

func (file *File) UnmarshalJSON(data []byte) error {
	var value struct {
		Size   *int64  `json:"size"`
		SHA256 *string `json:"sha256"`
	}
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	if value.Size == nil || value.SHA256 == nil {
		return fmt.Errorf("inventory entries require size and sha256")
	}
	file.Size, file.SHA256 = *value.Size, *value.SHA256
	return nil
}

type Descriptor struct {
	Schema        int             `json:"schema"`
	Kind          string          `json:"kind"`
	Name          string          `json:"name"`
	Version       string          `json:"version"`
	Architecture  string          `json:"architecture"`
	RuntimeABI    string          `json:"runtimeABI"`
	RootfsSHA256  string          `json:"rootfsSha256"`
	BootContracts []string        `json:"bootContracts"`
	Provides      []string        `json:"provides"`
	Requires      []string        `json:"requires"`
	Features      []string        `json:"features"`
	Source        map[string]any  `json:"source"`
	Files         map[string]File `json:"files"`
}

func Hash(filename string) (string, error) {
	f, err := os.Open(filename)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func read(filename string, target any) error {
	data, err := os.ReadFile(filename)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, target)
}

func capabilities(values []string) bool {
	if values == nil {
		return false
	}
	seen := map[string]bool{}
	for _, value := range values {
		if !namePattern.MatchString(value) || seen[value] {
			return false
		}
		seen[value] = true
	}
	return true
}

func Verify(directory string) (*Descriptor, string, error) {
	info, err := os.Lstat(directory)
	if err != nil || !info.IsDir() {
		return nil, "", fmt.Errorf("component must be a directory, not a symlink: %s", directory)
	}
	// Walk before opening the descriptor: even metadata symlinks are forbidden.
	actual := map[string]bool{}
	err = filepath.WalkDir(directory, func(filename string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		if !entry.Type().IsRegular() {
			return fmt.Errorf("non-regular component entry: %s", filename)
		}
		relative, err := filepath.Rel(directory, filename)
		if err != nil {
			return err
		}
		actual[filepath.ToSlash(relative)] = true
		return nil
	})
	if err != nil {
		return nil, "", err
	}
	var metadata Descriptor
	if err := read(filepath.Join(directory, "component.json"), &metadata); err != nil {
		return nil, "", err
	}
	if metadata.Schema != 2 || (metadata.Kind != "rootfs" && metadata.Kind != "bsp") || !namePattern.MatchString(metadata.Name) || !versionPattern.MatchString(metadata.Version) || metadata.Architecture != "arm64" {
		return nil, "", fmt.Errorf("invalid Pantavisor schema-2 component identity")
	}
	if !capabilities(metadata.Provides) || (metadata.Kind == "rootfs" && !capabilities(metadata.Requires)) {
		return nil, "", fmt.Errorf("invalid component capabilities")
	}
	commit, commitOK := metadata.Source["commit"].(string)
	tree, treeOK := metadata.Source["treeSha256"].(string)
	repository, repositoryOK := metadata.Source["repository"].(string)
	_, dirtyOK := metadata.Source["dirty"].(bool)
	if !commitOK || commit == "" || !treeOK || !hashPattern.MatchString(tree) || !repositoryOK || repository == "" || !dirtyOK {
		return nil, "", fmt.Errorf("invalid build source identity")
	}
	if len(actual) != len(metadata.Files)+1 || !actual["component.json"] {
		return nil, "", fmt.Errorf("unexpected or missing component files")
	}
	for name, expected := range metadata.Files {
		if name == "component.json" || name == "." || !fs.ValidPath(name) || path.Clean(name) != name || strings.ContainsAny(name, "\x00\r\n\\") || !actual[name] || expected.Size < 0 || !hashPattern.MatchString(expected.SHA256) {
			return nil, "", fmt.Errorf("invalid component inventory entry %q", name)
		}
		filename := filepath.Join(directory, filepath.FromSlash(name))
		stat, err := os.Stat(filename)
		if err != nil {
			return nil, "", err
		}
		checksum, err := Hash(filename)
		if err != nil || stat.Size() != expected.Size || checksum != expected.SHA256 {
			return nil, "", fmt.Errorf("component integrity failure: %s", name)
		}
	}
	required := []string{"build-provenance.json", "sbom.spdx.tar.zst"}
	if metadata.Kind == "rootfs" {
		required = append(required, "rootfs.squashfs", "rootfs.tar.gz", "runtime.json", "lxc.conf.in", "packages.manifest")
	} else {
		required = append(required, "bsp.json", "kernel.img", "initramfs-base.cpio.gz", "vendor-root.tar.gz", "selection.json")
	}
	for _, name := range required {
		if !actual[name] {
			return nil, "", fmt.Errorf("missing component payload %s", name)
		}
	}
	var provenance struct {
		Schema int            `json:"schema"`
		Source map[string]any `json:"source"`
	}
	if err := read(filepath.Join(directory, "build-provenance.json"), &provenance); err != nil {
		return nil, "", err
	}
	if provenance.Schema != 1 || len(provenance.Source) == 0 {
		return nil, "", fmt.Errorf("invalid build provenance")
	}
	for _, field := range []string{"commit", "dirty", "treeSha256"} {
		if !reflect.DeepEqual(provenance.Source[field], metadata.Source[field]) {
			return nil, "", fmt.Errorf("build provenance source mismatch: %s", field)
		}
	}
	for key, value := range provenance.Source {
		if !reflect.DeepEqual(metadata.Source[key], value) {
			return nil, "", fmt.Errorf("build provenance source mismatch: %s", key)
		}
	}
	if metadata.Kind == "rootfs" {
		if metadata.RuntimeABI == "" || metadata.Features == nil {
			return nil, "", fmt.Errorf("rootfs requires runtimeABI and features")
		}
		var runtime struct {
			Schema       int      `json:"schema"`
			ID           string   `json:"id"`
			Architecture string   `json:"architecture"`
			Provides     []string `json:"provides"`
			Requires     []string `json:"requires"`
		}
		if err := read(filepath.Join(directory, "runtime.json"), &runtime); err != nil {
			return nil, "", err
		}
		if runtime.Provides == nil {
			runtime.Provides = []string{}
		}
		if runtime.Schema != 1 || runtime.ID != metadata.RuntimeABI || runtime.Architecture != metadata.Architecture || !reflect.DeepEqual(runtime.Provides, metadata.Provides) || !reflect.DeepEqual(runtime.Requires, metadata.Requires) || metadata.RootfsSHA256 != metadata.Files["rootfs.squashfs"].SHA256 {
			return nil, "", fmt.Errorf("rootfs runtime contract mismatch")
		}
	} else {
		var bsp struct {
			BootContracts []string `json:"bootContracts"`
		}
		if err := read(filepath.Join(directory, "bsp.json"), &bsp); err != nil {
			return nil, "", err
		}
		if len(bsp.BootContracts) == 0 || !reflect.DeepEqual(bsp.BootContracts, metadata.BootContracts) {
			return nil, "", fmt.Errorf("BSP boot contract mismatch")
		}
	}
	digest, err := Hash(filepath.Join(directory, "component.json"))
	return &metadata, "sha256:" + digest, err
}
