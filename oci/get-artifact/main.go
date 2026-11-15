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
	MediaType string `json:"media_type,omitempty"`
}

func validate(files []payload) error {
	if len(files) == 0 {
		return fmt.Errorf("expected payload list is required")
	}
	seen := map[string]bool{}
	for _, file := range files {
		if file.Path == "" || file.Path == "." || file.Path == ".." ||
			filepath.Base(file.Path) != file.Path || strings.ContainsAny(file.Path, ":\\\r\n") ||
			strings.HasPrefix(file.Path, "-") || seen[file.Path] ||
			!regexp.MustCompile(`^[a-f0-9]{64}$`).MatchString(file.SHA256) {
			return fmt.Errorf("invalid or duplicate payload %q", file.Path)
		}
		seen[file.Path] = true
	}
	return nil
}

func verify(directory string, files []payload) error {
	if err := validate(files); err != nil {
		return err
	}
	entries, err := os.ReadDir(directory)
	if err != nil {
		return err
	}
	if len(entries) != len(files) {
		return fmt.Errorf("downloaded payload set differs from expected files")
	}
	for _, file := range files {
		path := filepath.Join(directory, file.Path)
		info, err := os.Lstat(path)
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("%s is not a regular payload", file.Path)
		}
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		h := sha256.New()
		_, hashErr := io.Copy(h, f)
		closeErr := f.Close()
		if hashErr != nil {
			return hashErr
		}
		if closeErr != nil {
			return closeErr
		}
		if hex.EncodeToString(h.Sum(nil)) != file.SHA256 {
			return fmt.Errorf("downloaded checksum mismatch for %s", file.Path)
		}
	}
	return nil
}

func run() error {
	reference := flag.String("reference", "", "immutable GHCR artifact reference")
	filesJSON := flag.String("files", "", "expected flat payloads and checksums")
	directory := flag.String("output", ".build/iso", "new workspace-relative output directory")
	username := flag.String("registry-username", "", "optional registry user")
	flag.Parse()
	if flag.NArg() != 0 || !regexp.MustCompile(`^ghcr\.io/[a-z0-9][a-z0-9._/-]+@sha256:[a-f0-9]{64}$`).MatchString(*reference) {
		return fmt.Errorf("immutable GHCR artifact reference required")
	}
	if *directory == "" || filepath.IsAbs(*directory) || filepath.Clean(*directory) == "." ||
		filepath.Clean(*directory) == ".." || strings.HasPrefix(filepath.Clean(*directory), "../") {
		return fmt.Errorf("output must be a new workspace-relative directory")
	}
	var files []payload
	decoder := json.NewDecoder(strings.NewReader(*filesJSON))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&files); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return fmt.Errorf("unexpected trailing payload JSON")
	}
	if err := validate(files); err != nil {
		return err
	}
	if _, err := os.Lstat(*directory); !os.IsNotExist(err) {
		return fmt.Errorf("output directory must not already exist")
	}
	private, err := os.MkdirTemp("", "oras-get-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(private)
	auth := filepath.Join(private, "auth.json")
	password := os.Getenv("INPUT_REGISTRY_PASSWORD")
	if password != "" {
		if *username == "" {
			return fmt.Errorf("registry username required with a password")
		}
		login := exec.Command("oras", "login", "ghcr.io", "--username", *username,
			"--password-stdin", "--registry-config", auth)
		login.Stdin = strings.NewReader(password)
		login.Stdout, login.Stderr = os.Stdout, os.Stderr
		if err := login.Run(); err != nil {
			return err
		}
	}
	if err := os.MkdirAll(*directory, 0755); err != nil {
		return err
	}
	pull := exec.Command("oras", "pull", *reference, "--output", *directory, "--registry-config", auth)
	pull.Stdout, pull.Stderr = os.Stdout, os.Stderr
	if err := pull.Run(); err != nil {
		return err
	}
	if err := verify(*directory, files); err != nil {
		return err
	}
	if err := workflow.Output("reference", *reference); err != nil {
		return err
	}
	return workflow.Output("directory", *directory)
}

func main() { workflow.Main("get-artifact 1", run) }
