package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/tdesaules/github-actions/shared/workflow"
)

type payload struct {
	Path      string `json:"path"`
	SHA256    string `json:"sha256"`
	MediaType string `json:"media_type"`
}

var digestPattern = regexp.MustCompile(`^[a-f0-9]{64}$`)
var mediaPattern = regexp.MustCompile(`^[a-z0-9.+-]+/[a-z0-9.+-]+$`)
var referencePattern = regexp.MustCompile(`^ghcr\.io/[a-z0-9][a-z0-9._/-]*:[a-zA-Z0-9_][a-zA-Z0-9_.-]{0,127}$`)

func checksum(path string) (string, error) {
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

func validatePayloads(directory string, files []payload) error {
	if len(files) == 0 {
		return fmt.Errorf("at least one payload is required")
	}
	seen := map[string]bool{}
	for _, f := range files {
		// Flat names keep ORAS extraction independent of directory traversal and
		// avoid platform-dependent interpretation of nested archive paths.
		if f.Path == "" || f.Path == "." || f.Path == ".." || filepath.Base(f.Path) != f.Path ||
			strings.ContainsAny(f.Path, ":\\\r\n") || strings.HasPrefix(f.Path, "-") || seen[f.Path] {
			return fmt.Errorf("invalid or duplicate payload name %q", f.Path)
		}
		seen[f.Path] = true
		if !digestPattern.MatchString(f.SHA256) || !mediaPattern.MatchString(f.MediaType) {
			return fmt.Errorf("invalid checksum or media type for %s", f.Path)
		}
		path := filepath.Join(directory, f.Path)
		info, err := os.Lstat(path)
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("%s is not a regular payload", f.Path)
		}
		if info.Size() > 10_000_000_000 {
			return fmt.Errorf("%s exceeds the conservative GHCR 10 GB layer limit", f.Path)
		}
		actual, err := checksum(path)
		if err != nil {
			return err
		}
		if actual != f.SHA256 {
			return fmt.Errorf("checksum mismatch for %s", f.Path)
		}
	}
	return nil
}

func oras(directory, auth string, args ...string) ([]byte, error) {
	cmd := exec.Command("oras", append(args, "--registry-config", auth)...)
	cmd.Dir = directory
	cmd.Stderr = os.Stderr
	return cmd.Output()
}

func run() error {
	directory := flag.String("directory", "", "qualified payload directory")
	filesJSON := flag.String("files", "", "JSON array of path, sha256 and media_type")
	reference := flag.String("reference", "", "GHCR repository and tag")
	artifactType := flag.String("artifact-type", "", "OCI artifact type")
	username := flag.String("registry-username", "", "registry user")
	flag.Parse()
	if flag.NArg() != 0 || *directory == "" || *username == "" || !referencePattern.MatchString(*reference) ||
		!mediaPattern.MatchString(*artifactType) {
		return fmt.Errorf("directory, username, GHCR reference and artifact type are required")
	}
	var files []payload
	decoder := json.NewDecoder(strings.NewReader(*filesJSON))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&files); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return fmt.Errorf("unexpected trailing payload JSON")
	}
	if err := validatePayloads(*directory, files); err != nil {
		return err
	}
	password := os.Getenv("INPUT_REGISTRY_PASSWORD")
	if password == "" {
		return fmt.Errorf("registry password is required")
	}
	private, err := os.MkdirTemp("", "oras-publish-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(private)
	auth := filepath.Join(private, "auth.json")
	login := exec.Command("oras", "login", "ghcr.io", "--username", *username,
		"--password-stdin", "--registry-config", auth)
	login.Stdin = strings.NewReader(password)
	login.Stdout, login.Stderr = os.Stdout, os.Stderr
	if err := login.Run(); err != nil {
		return err
	}
	args := []string{"push", "--format", "json", "--artifact-type", *artifactType, *reference}
	for _, f := range files {
		args = append(args, f.Path+":"+f.MediaType)
	}
	result, err := oras(*directory, auth, args...)
	if err != nil {
		return err
	}
	var published struct {
		Digest string `json:"digest"`
	}
	if err := json.Unmarshal(result, &published); err != nil {
		return err
	}
	if !strings.HasPrefix(published.Digest, "sha256:") ||
		!digestPattern.MatchString(strings.TrimPrefix(published.Digest, "sha256:")) {
		return fmt.Errorf("ORAS did not return a valid manifest digest")
	}
	immutable := strings.SplitN(*reference, ":", 2)[0] + "@" + published.Digest
	download := filepath.Join(private, "download")
	if err := os.Mkdir(download, 0700); err != nil {
		return err
	}
	if _, err := oras(download, auth, "pull", immutable); err != nil {
		return err
	}
	if err := validatePayloads(download, files); err != nil {
		return fmt.Errorf("published artifact failed round-trip verification: %w", err)
	}
	if err := validatePayloads(*directory, files); err != nil {
		return fmt.Errorf("source payload changed during publication: %w", err)
	}
	if err := workflow.Output("digest", published.Digest); err != nil {
		return err
	}
	return workflow.Output("reference", immutable)
}

func main() { workflow.Main("publish-artifact 1", run) }
