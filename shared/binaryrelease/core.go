// Package binaryrelease implements the source-bound Linux binary release protocol.
package binaryrelease

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

var stable = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$`)
var sha = regexp.MustCompile(`^[0-9a-f]{40}$`)
var digest = regexp.MustCompile(`^[0-9a-f]{64}$`)
var decimal = regexp.MustCompile(`^[1-9][0-9]*$`)
var archiveName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*\.tar\.gz$`)

const Component = "niri-tonic-files"
const markerPrefix = "binary-release-v1\n"

type Receipt struct {
	SchemaVersion int    `json:"schema_version"`
	SourceSHA     string `json:"source_sha"`
	Version       string `json:"version"`
	Component     string `json:"component"`
	Arch          string `json:"arch"`
	Archive       string `json:"archive"`
	SHA256        string `json:"sha256"`
	RunID         string `json:"run_id"`
	RunAttempt    string `json:"run_attempt"`
	Tested        bool   `json:"tested"`
}
type Qualified struct {
	Receipt
	Path string `json:"-"`
}
type ArtifactBinding struct {
	ID      int64  `json:"id"`
	Arch    string `json:"arch"`
	Digest  string `json:"digest"`
	SHA256  string `json:"sha256"`
	Archive string `json:"archive"`
}
type Marker struct {
	SchemaVersion int               `json:"schema_version"`
	SourceSHA     string            `json:"source_sha"`
	Version       string            `json:"version"`
	RunID         string            `json:"run_id"`
	RunAttempt    string            `json:"run_attempt"`
	Artifacts     []ArtifactBinding `json:"artifacts"`
}

func Input(name, fallback string) string {
	if value, ok := cliInputs[name]; ok {
		return value
	}
	if v := os.Getenv("INPUT_" + strings.ToUpper(name)); v != "" {
		return v
	}
	if v := os.Getenv("INPUT_" + strings.ToUpper(strings.ReplaceAll(name, "-", "_"))); v != "" {
		return v
	}
	return fallback
}
func Output(name string, value any) error {
	var s string
	if v, ok := value.(string); ok {
		s = v
	} else {
		b, e := json.Marshal(value)
		if e != nil {
			return e
		}
		s = string(b)
	}
	if strings.ContainsAny(s, "\r\n") {
		return fmt.Errorf("multiline output rejected")
	}
	f, e := os.OpenFile(os.Getenv("GITHUB_OUTPUT"), os.O_APPEND|os.O_WRONLY, 0600)
	if e != nil {
		return e
	}
	defer f.Close()
	_, e = fmt.Fprintf(f, "%s=%s\n", name, s)
	return e
}
func Git(args ...string) (string, error) {
	c := gitCommand(args...)
	b, e := c.Output()
	if e != nil {
		return "", fmt.Errorf("git %s failed", args[0])
	}
	return strings.TrimSpace(string(b)), nil
}
func gitRaw(args ...string) ([]byte, error) {
	b, e := gitCommand(args...).Output()
	if e != nil {
		return nil, fmt.Errorf("git %s failed", args[0])
	}
	return b, nil
}

// Trust only the explicit mounted workspace, whose owner differs from the Docker
// action UID on hosted runners. Never install a wildcard global safe.directory.
func gitCommand(args ...string) *exec.Cmd {
	dir, _ := os.Getwd()
	return exec.Command("git", append([]string{"-c", "safe.directory=" + dir}, args...)...)
}
func Head(expected string) error {
	h, e := Git("rev-parse", "HEAD")
	if e != nil {
		return e
	}
	if !sha.MatchString(expected) || h != expected {
		return fmt.Errorf("source SHA must equal immutable HEAD")
	}
	return nil
}
func Hash(path string) (string, error) {
	f, e := os.Open(path)
	if e != nil {
		return "", e
	}
	defer f.Close()
	h := sha256.New()
	_, e = io.Copy(h, f)
	return hex.EncodeToString(h.Sum(nil)), e
}
func WriteJSON(path string, v any) error {
	b, e := json.MarshalIndent(v, "", "  ")
	if e != nil {
		return e
	}
	if e = os.MkdirAll(filepath.Dir(path), 0755); e != nil {
		return e
	}
	return os.WriteFile(path, append(b, '\n'), 0644)
}
func Decode(path string, v any) error {
	b, e := os.ReadFile(path)
	if e != nil {
		return e
	}
	if len(b) > 1024*1024 {
		return fmt.Errorf("JSON too large")
	}
	return json.Unmarshal(b, v)
}
func Verify(dir, source, version, run, attempt string) ([]Qualified, error) {
	if !sha.MatchString(source) || !stable.MatchString(version) || !decimal.MatchString(run) || !decimal.MatchString(attempt) {
		return nil, fmt.Errorf("invalid expected qualification identity")
	}
	found := map[string]Qualified{}
	archives := map[string]bool{}
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, e error) error {
		if e != nil {
			return e
		}
		if d.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("symlink in qualification directory")
		}
		if !d.IsDir() && strings.HasSuffix(path, ".tar.gz") {
			archives[filepath.Clean(path)] = true
		}
		if d.IsDir() || !strings.HasSuffix(path, ".receipt.json") {
			return nil
		}
		var r Receipt
		if e = Decode(path, &r); e != nil {
			return e
		}
		if r.SchemaVersion != 1 || r.Component != Component || r.SourceSHA != source || r.Version != version || r.RunID != run || r.RunAttempt != attempt || !r.Tested || (r.Arch != "amd64" && r.Arch != "arm64") {
			return fmt.Errorf("invalid qualification receipt")
		}
		if _, ok := found[r.Arch]; ok {
			return fmt.Errorf("duplicate architecture")
		}
		if !archiveName.MatchString(r.Archive) || !digest.MatchString(r.SHA256) {
			return fmt.Errorf("invalid archive identity")
		}
		archive := filepath.Join(filepath.Dir(path), r.Archive)
		st, e := os.Lstat(archive)
		if e != nil {
			return e
		}
		if !st.Mode().IsRegular() {
			return fmt.Errorf("archive must be regular file")
		}
		sum, e := Hash(archive)
		if e != nil {
			return e
		}
		if sum != r.SHA256 {
			return fmt.Errorf("archive checksum mismatch")
		}
		found[r.Arch] = Qualified{r, archive}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if len(found) != 2 {
		return nil, fmt.Errorf("complete amd64/arm64 qualification required")
	}
	if found["amd64"].Archive == found["arm64"].Archive {
		return nil, fmt.Errorf("archive names collide")
	}
	for _, q := range found {
		delete(archives, filepath.Clean(q.Path))
	}
	if len(archives) != 0 {
		return nil, fmt.Errorf("unexpected archive outside qualified set")
	}
	return []Qualified{found["amd64"], found["arm64"]}, nil
}
func VersionCompare(a, b string) int {
	aa := strings.Split(a, ".")
	bb := strings.Split(b, ".")
	for i := 0; i < 3; i++ {
		x, y := aa[i], bb[i]
		if len(x) < len(y) || (len(x) == len(y) && x < y) {
			return -1
		}
		if len(x) > len(y) || (len(x) == len(y) && x > y) {
			return 1
		}
	}
	return 0
}
func ValidateMarker(m Marker) error {
	if m.SchemaVersion != 1 || !sha.MatchString(m.SourceSHA) || !stable.MatchString(m.Version) || !decimal.MatchString(m.RunID) || !decimal.MatchString(m.RunAttempt) || len(m.Artifacts) != 2 {
		return fmt.Errorf("invalid release marker")
	}
	seen := map[string]bool{}
	ids := map[int64]bool{}
	for _, a := range m.Artifacts {
		if (a.Arch != "amd64" && a.Arch != "arm64") || seen[a.Arch] || a.ID <= 0 || ids[a.ID] || !digest.MatchString(a.SHA256) || !strings.HasPrefix(a.Digest, "sha256:") || !digest.MatchString(strings.TrimPrefix(a.Digest, "sha256:")) || !archiveName.MatchString(a.Archive) {
			return fmt.Errorf("invalid artifact binding")
		}
		seen[a.Arch] = true
		ids[a.ID] = true
	}
	return nil
}
func MatchBindings(q []Qualified, m Marker) error {
	for _, r := range q {
		ok := false
		for _, a := range m.Artifacts {
			if a.Arch == r.Arch && a.SHA256 == r.SHA256 && a.Archive == r.Archive {
				ok = true
			}
		}
		if !ok {
			return fmt.Errorf("archive not authorized by annotated release tag")
		}
	}
	return nil
}
