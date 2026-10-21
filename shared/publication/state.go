// Package publication implements fixed catalogue publication stages.
package publication

import (
	"archive/tar"
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/tdesaules/github-actions/shared/catalog"
	"github.com/tdesaules/github-actions/shared/workflow"
)

type Archive struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
}
type Image struct {
	Reference string             `json:"reference"`
	Platforms map[string]string  `json:"platforms"`
	Archives  map[string]Archive `json:"archives,omitempty"`
	Reused    bool               `json:"reused"`
	Published bool               `json:"published"`
	Signed    bool               `json:"signed"`
}
type State struct {
	Version  string            `json:"version"`
	Revision string            `json:"revision"`
	Images   map[string]*Image `json:"images"`
}
type ReleasePlan struct {
	Base     string `json:"base"`
	Version  string `json:"version"`
	Previous string `json:"previous"`
	Changed  bool   `json:"changed"`
}

var Stable = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$`)
var SemVer = regexp.MustCompile(`^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)(?:-((?:0|[1-9]\d*|\d*[A-Za-z-][0-9A-Za-z-]*)(?:\.(?:0|[1-9]\d*|\d*[A-Za-z-][0-9A-Za-z-]*))*))?(?:\+([0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*))?$`)
var Digest = regexp.MustCompile(`^sha256:[a-f0-9]{64}$`)

func Hash(data []byte) string { return fmt.Sprintf("sha256:%x", sha256.Sum256(data)) }
func Revision() (string, error) {
	v, err := workflow.Git("rev-parse", "HEAD")
	return strings.TrimSpace(v), err
}
func Checksum(path string) (string, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("not a regular archive: %s", path)
	}
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err = io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
func ArchiveDigest(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	r := tar.NewReader(f)
	for {
		h, err := r.Next()
		if err != nil {
			return "", err
		}
		if h.Name != "index.json" {
			continue
		}
		var index struct {
			Manifests []struct {
				Digest string `json:"digest"`
			} `json:"manifests"`
		}
		if err = json.NewDecoder(io.LimitReader(r, 1<<20)).Decode(&index); err != nil {
			return "", err
		}
		if len(index.Manifests) != 1 {
			return "", fmt.Errorf("archive needs one manifest")
		}
		return index.Manifests[0].Digest, nil
	}
}
func Children(raw []byte) (map[string]string, error) {
	var index struct {
		Manifests []struct {
			Digest   string `json:"digest"`
			Platform struct {
				Architecture string `json:"architecture"`
				OS           string `json:"os"`
			} `json:"platform"`
		} `json:"manifests"`
	}
	if err := json.Unmarshal(raw, &index); err != nil {
		return nil, err
	}
	if len(index.Manifests) != 2 {
		return nil, fmt.Errorf("index must contain exactly two platforms")
	}
	result := map[string]string{}
	for _, m := range index.Manifests {
		arch := m.Platform.Architecture
		if m.Platform.OS != "linux" || (arch != "amd64" && arch != "arm64") || !Digest.MatchString(m.Digest) || result[arch] != "" {
			return nil, fmt.Errorf("invalid or duplicate platform")
		}
		result[arch] = m.Digest
	}
	return result, nil
}
func Raw(reference string) ([]byte, error) {
	cmd := exec.Command("skopeo", "inspect", "--raw", "docker://"+reference)
	cmd.Stderr = os.Stderr
	return cmd.Output()
}

func rawOptional(reference string) ([]byte, error) {
	cmd := exec.Command("skopeo", "inspect", "--raw", "docker://"+reference)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	data, err := cmd.Output()
	if err == nil {
		return data, nil
	}
	message := strings.ToLower(stderr.String())
	if strings.Contains(message, "manifest unknown") || strings.Contains(message, "name unknown") {
		return nil, nil
	}
	return nil, fmt.Errorf("registry inspection failed: %s: %w", stderr.String(), err)
}
func Auth() (func(), error) {
	token, user := os.Getenv("INPUT_REGISTRY-PASSWORD"), os.Getenv("INPUT_REGISTRY-USERNAME")
	if token == "" && user == "" {
		return func() {}, nil
	}
	if token == "" || user == "" {
		return nil, fmt.Errorf("registry username and token must be provided together")
	}
	dir, err := os.MkdirTemp("", "catalog-auth-")
	if err != nil {
		return nil, err
	}
	data, _ := json.Marshal(map[string]any{"auths": map[string]any{"ghcr.io": map[string]string{"auth": base64.StdEncoding.EncodeToString([]byte(user + ":" + token))}}})
	if err = os.WriteFile(filepath.Join(dir, "config.json"), data, 0600); err != nil {
		os.RemoveAll(dir)
		return nil, err
	}
	os.Setenv("DOCKER_CONFIG", dir)
	os.Setenv("REGISTRY_AUTH_FILE", filepath.Join(dir, "config.json"))
	return func() { os.RemoveAll(dir) }, nil
}
func Load(directory string) (State, error) {
	var state State
	if err := catalog.Read(filepath.Join(directory, "publication.json"), &state); err != nil {
		return state, err
	}
	revision, err := Revision()
	if err != nil {
		return state, err
	}
	if state.Revision != revision {
		return state, fmt.Errorf("publication source differs from checkout")
	}
	if !Stable.MatchString(state.Version) && state.Version != "0.0.0-bootstrap" {
		return state, fmt.Errorf("invalid release version")
	}
	images, err := catalog.Discover()
	if err != nil {
		return state, err
	}
	if len(images) != len(state.Images) {
		return state, fmt.Errorf("publication does not cover current catalogue")
	}
	for _, owner := range images {
		image := state.Images[owner.Name]
		if image == nil || !catalog.Pinned.MatchString(image.Reference) || len(image.Platforms) != 2 {
			return state, fmt.Errorf("invalid publication for %s", owner.Name)
		}
		if !strings.HasSuffix(strings.Split(image.Reference, "@")[0], "/github-actions/"+owner.Name) {
			return state, fmt.Errorf("wrong publication owner")
		}
		for _, arch := range []string{"amd64", "arm64"} {
			if !Digest.MatchString(image.Platforms[arch]) {
				return state, fmt.Errorf("missing platform digest")
			}
		}
		if image.Reused && image.Reference != owner.Reference {
			return state, fmt.Errorf("reused reference changed")
		}
	}
	return state, nil
}
func Save(directory string, state State) error {
	return catalog.Write(filepath.Join(directory, "publication.json"), state)
}
func ArchivePaths(directory string, image *Image) ([]string, error) {
	paths := []string{}
	if len(image.Archives) != 2 {
		return nil, fmt.Errorf("both archives required")
	}
	for _, arch := range []string{"amd64", "arm64"} {
		a := image.Archives[arch]
		if a.Path == "" || filepath.IsAbs(a.Path) || strings.HasPrefix(filepath.Clean(a.Path), "..") {
			return nil, fmt.Errorf("invalid archive path")
		}
		p := filepath.Join(directory, a.Path)
		sum, err := Checksum(p)
		if err != nil {
			return nil, err
		}
		if sum != a.SHA256 {
			return nil, fmt.Errorf("archive checksum mismatch: %s", p)
		}
		paths = append(paths, p)
	}
	return paths, nil
}
