// Package atomicrelease implements source-bound Atomic OS release controls.
package atomicrelease

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

var Stable = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$`)
var sha = regexp.MustCompile(`^[a-f0-9]{40}$`)
var hash = regexp.MustCompile(`^[a-f0-9]{64}$`)
var digest = regexp.MustCompile(`^sha256:[a-f0-9]{64}$`)
var repositoryName = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*/[a-z0-9][a-z0-9._-]*$`)

type Receipt struct {
	Files           []Payload         `json:"files,omitempty"`
	SchemaVersion   int               `json:"schema_version"`
	Component       string            `json:"component"`
	Arch            string            `json:"arch"`
	SourceSHA       string            `json:"source_sha"`
	Version         string            `json:"version"`
	Reference       string            `json:"reference,omitempty"`
	Archive         string            `json:"archive"`
	SHA256          string            `json:"sha256"`
	ManifestDigest  string            `json:"manifest_digest"`
	OSArchiveSHA256 string            `json:"os_archive_sha256,omitempty"`
	Size            int64             `json:"size,omitempty"`
	Tested          bool              `json:"tested"`
	Qualification   string            `json:"qualification"`
	BootTested      bool              `json:"boot_tested"`
	RunID           string            `json:"run_id"`
	RunAttempt      string            `json:"run_attempt"`
	BuildRunID      string            `json:"build_run_id,omitempty"`
	BuildRunAttempt string            `json:"build_run_attempt,omitempty"`
	Checks          map[string]string `json:"checks,omitempty"`
	Packages        []string          `json:"packages,omitempty"`
}

type Bundle struct {
	AMD64 Receipt `json:"amd64"`
	ARM64 Receipt `json:"arm64"`
	ISO   Receipt `json:"iso"`
}

type Evidence struct {
	SchemaVersion int       `json:"schema_version"`
	Version       string    `json:"version"`
	SourceSHA     string    `json:"source_sha"`
	Image         string    `json:"image"`
	ISO           string    `json:"iso"`
	ISOFile       string    `json:"iso_file"`
	ISOSHA256     string    `json:"iso_sha256"`
	Architectures []string  `json:"architectures"`
	Qualified     Bundle    `json:"qualified"`
	ISOFiles      []Payload `json:"iso_files"`
}

type Payload struct {
	Path      string `json:"path"`
	SHA256    string `json:"sha256"`
	MediaType string `json:"media_type"`
}

func Decode(path string, v any) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, v)
}
func Write(path string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	if err = os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	return os.WriteFile(path, append(b, '\n'), 0644)
}
func checksum(path string) (string, int64, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return "", 0, err
	}
	if !info.Mode().IsRegular() {
		return "", 0, fmt.Errorf("payload must be a regular file")
	}
	f, err := os.Open(path)
	if err != nil {
		return "", 0, err
	}
	defer f.Close()
	h := sha256.New()
	n, err := io.Copy(h, f)
	return fmt.Sprintf("%x", h.Sum(nil)), n, err
}
func flat(name string) bool {
	return name != "" && name != "." && name != ".." && filepath.Base(name) == name && !strings.ContainsAny(name, "\\\r\n")
}
func compatible(got, current string) bool {
	a, e := strconv.Atoi(got)
	b, f := strconv.Atoi(current)
	return e == nil && f == nil && a > 0 && b >= a
}
func ValidateReceipt(r Receipt, component, arch, source, version, run, attempt string) error {
	if r.SchemaVersion != 1 || r.Component != component || r.Arch != arch || r.SourceSHA != source ||
		!sha.MatchString(source) || r.Version != version || !Stable.MatchString(version) ||
		!r.Tested || r.BootTested || !flat(r.Archive) || !hash.MatchString(r.SHA256) ||
		!digest.MatchString(r.ManifestDigest) || r.RunID != run || !regexp.MustCompile(`^[1-9][0-9]*$`).MatchString(run) ||
		!compatible(r.RunAttempt, attempt) {
		return fmt.Errorf("invalid %s/%s qualification identity, run or integrity", component, arch)
	}
	if component == "os" && (r.Qualification != "userspace" || len(r.Checks) == 0 || len(r.Packages) == 0) {
		return fmt.Errorf("missing OS functional evidence")
	}
	if component == "os" {
		// Values are command output and may legitimately be empty on success.
		for _, name := range []string{
			"Fedora version", "Niri configuration", "desktop and keyring packages",
			"PAM dynamic dependencies", "enabled greeter", "enabled GPU profile",
			"GPU helper syntax", "GPU memory ceiling",
		} {
			if _, ok := r.Checks[name]; !ok {
				return fmt.Errorf("missing OS functional check: %s", name)
			}
		}
	}
	if component == "iso" && (r.Qualification != "installer-media" || r.Size <= 0 || !strings.HasSuffix(r.Archive, ".iso")) {
		return fmt.Errorf("missing ISO media evidence")
	}
	return nil
}
func ValidateBundle(b Bundle, source, version, run, attempt string) error {
	if b.ISO.BuildRunID != run || !compatible(b.ISO.BuildRunAttempt, attempt) {
		return fmt.Errorf("preparation ISO build must belong to this run and a compatible attempt")
	}
	return validateBundle(b, source, version, run, attempt, b.AMD64.SHA256)
}
func validateBundle(b Bundle, source, version, run, attempt, originalAMD64 string) error {
	for _, item := range []struct {
		r               Receipt
		component, arch string
	}{{b.AMD64, "os", "amd64"}, {b.ARM64, "os", "arm64"}, {b.ISO, "iso", "amd64"}} {
		if err := ValidateReceipt(item.r, item.component, item.arch, source, version, run, attempt); err != nil {
			return err
		}
	}
	if b.ISO.OSArchiveSHA256 != originalAMD64 || b.ISO.ManifestDigest != b.AMD64.ManifestDigest {
		return fmt.Errorf("ISO must embed the exact qualified AMD64 OS")
	}
	return nil
}

// Verify accepts earlier successful attempts of the same run, never future attempts
// or receipts from another workflow run.
func Verify(directory, source, version, run, attempt string) (Bundle, error) {
	return verify(directory, source, version, run, attempt, "")
}
func verify(directory, source, version, run, attempt, originalAMD64 string) (Bundle, error) {
	var b Bundle
	entries, err := os.ReadDir(directory)
	if err != nil {
		return b, err
	}
	for _, entry := range entries {
		if entry.IsDir() && entry.Name() != "amd64" && entry.Name() != "arm64" {
			return b, fmt.Errorf("unexpected platform in release bundle")
		}
	}
	for _, arch := range []string{"amd64", "arm64"} {
		info, err := os.Lstat(filepath.Join(directory, arch))
		if err != nil {
			return b, err
		}
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return b, fmt.Errorf("platform directory must not be a symlink")
		}
	}
	for _, item := range []struct {
		r          *Receipt
		arch, name string
	}{
		{&b.AMD64, "amd64", "qualification.json"}, {&b.ARM64, "arm64", "qualification.json"}, {&b.ISO, "amd64", "iso-qualification.json"},
	} {
		if err := Decode(filepath.Join(directory, item.arch, item.name), item.r); err != nil {
			return b, err
		}
	}
	if originalAMD64 == "" {
		if err := ValidateBundle(b, source, version, run, attempt); err != nil {
			return b, err
		}
		originalAMD64 = b.AMD64.SHA256
	}
	if err := validateBundle(b, source, version, run, attempt, originalAMD64); err != nil {
		return b, err
	}
	for _, r := range []Receipt{b.AMD64, b.ARM64, b.ISO} {
		sum, size, err := checksum(filepath.Join(directory, r.Arch, r.Archive))
		if err != nil {
			return b, err
		}
		if sum != r.SHA256 || (r.Component == "iso" && size != r.Size) {
			return b, fmt.Errorf("tampered %s/%s payload", r.Component, r.Arch)
		}
	}
	sidecar, err := os.ReadFile(filepath.Join(directory, "amd64", b.ISO.Archive+"-CHECKSUM"))
	if err != nil {
		return b, err
	}
	if string(sidecar) != b.ISO.SHA256+"  "+b.ISO.Archive+"\n" {
		return b, fmt.Errorf("invalid ISO checksum sidecar")
	}
	return b, nil
}
func ValidateEvidence(e Evidence, version, repository string) error {
	if !repositoryName.MatchString(repository) || e.SchemaVersion != 1 || e.Version != version ||
		!Stable.MatchString(version) || !sha.MatchString(e.SourceSHA) ||
		!strings.HasPrefix(e.Image, "ghcr.io/"+repository+"@") || !digest.MatchString(strings.TrimPrefix(e.Image, "ghcr.io/"+repository+"@")) ||
		!strings.HasPrefix(e.ISO, "ghcr.io/"+repository+"-iso@") || !digest.MatchString(strings.TrimPrefix(e.ISO, "ghcr.io/"+repository+"-iso@")) ||
		len(e.Architectures) != 2 || e.Architectures[0] != "amd64" || e.Architectures[1] != "arm64" {
		return fmt.Errorf("invalid source-bound immutable release evidence")
	}
	// Receipts may originate in earlier successful attempts of the same preparation.
	attempt := e.Qualified.AMD64.RunAttempt
	for _, r := range []Receipt{e.Qualified.ARM64, e.Qualified.ISO} {
		if compatible(attempt, r.RunAttempt) {
			attempt = r.RunAttempt
		}
	}
	if err := ValidateBundle(e.Qualified, e.SourceSHA, e.Version, e.Qualified.AMD64.RunID, attempt); err != nil {
		return err
	}
	if e.ISOFile != e.Qualified.ISO.Archive || e.ISOSHA256 != e.Qualified.ISO.SHA256 {
		return fmt.Errorf("ISO evidence mismatch")
	}
	return ValidatePayloads(e.ISOFiles, e.Qualified.ISO)
}

func ValidatePayloads(files []Payload, r Receipt) error {
	if len(files) != 3 {
		return fmt.Errorf("ISO publication requires ISO, checksum and original build receipt")
	}
	seen := map[string]bool{}
	types := map[string]int{}
	for _, f := range files {
		if !flat(f.Path) || seen[f.Path] || !hash.MatchString(f.SHA256) {
			return fmt.Errorf("invalid or duplicate ISO payload")
		}
		seen[f.Path] = true
		types[f.MediaType]++
		switch f.MediaType {
		case "application/x-iso9660-image":
			if f.Path != r.Archive || f.SHA256 != r.SHA256 {
				return fmt.Errorf("ISO payload identity mismatch")
			}
		case "text/plain":
			if f.Path != r.Archive+"-CHECKSUM" {
				return fmt.Errorf("ISO sidecar name mismatch")
			}
		case "application/json":
			if f.Path != r.Archive+".build.json" {
				return fmt.Errorf("original untested build receipt must be named after the ISO")
			}
		default:
			return fmt.Errorf("unexpected ISO payload media type")
		}
	}
	for _, typ := range []string{"application/x-iso9660-image", "text/plain", "application/json"} {
		if types[typ] != 1 {
			return fmt.Errorf("incomplete ISO payload set")
		}
	}
	return nil
}
func VerifyPayloads(directory string, files []Payload, r Receipt) error {
	if err := ValidatePayloads(files, r); err != nil {
		return err
	}
	for _, f := range files {
		path := filepath.Join(directory, "amd64", f.Path)
		sum, _, err := checksum(path)
		if err != nil {
			return err
		}
		if sum != f.SHA256 {
			return fmt.Errorf("ISO publication payload checksum mismatch")
		}
		if f.MediaType == "application/json" {
			var build Receipt
			if err = Decode(path, &build); err != nil {
				return err
			}
			if build.SchemaVersion != 1 || build.Tested || build.Component != "iso" || build.Arch != "amd64" ||
				build.SourceSHA != r.SourceSHA || build.Version != r.Version || build.Archive != r.Archive ||
				build.SHA256 != r.SHA256 || build.Size != r.Size || build.OSArchiveSHA256 != r.OSArchiveSHA256 ||
				build.RunID != r.BuildRunID || build.RunAttempt != r.BuildRunAttempt {
				return fmt.Errorf("ISO build receipt differs from qualification")
			}
		}
	}
	return nil
}
