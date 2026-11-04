package component

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"syscall"
)

var registryPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9.-]*(?::[0-9]{1,5})?/[a-z0-9][a-z0-9._/-]*[a-z0-9]$`)
var referencePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9.-]*(?::[0-9]{1,5})?/[a-z0-9][a-z0-9._/-]*[a-z0-9]@sha256:[a-f0-9]{64}$`)

func auth(registry, username, token string) (string, func(), error) {
	dir, err := os.MkdirTemp("", "oras-auth-")
	if err != nil {
		return "", nil, err
	}
	cleanup := func() { os.RemoveAll(dir) }
	auths := map[string]any{}
	if token != "" {
		if username == "" {
			cleanup()
			return "", nil, fmt.Errorf("registry username required with token")
		}
		auths[registry] = map[string]string{"auth": base64.StdEncoding.EncodeToString([]byte(username + ":" + token))}
	}
	data, _ := json.Marshal(map[string]any{"auths": auths})
	file := filepath.Join(dir, "config.json")
	if err := os.WriteFile(file, data, 0600); err != nil {
		cleanup()
		return "", nil, err
	}
	return file, cleanup, nil
}
func oras(directory, config string, args ...string) (string, error) {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	command := append([]string{args[0], "--registry-config", config}, args[1:]...)
	if args[0] == "cp" {
		command = append([]string{"cp", "--from-registry-config", config, "--to-registry-config", config}, args[1:]...)
	}
	cmd := exec.CommandContext(ctx, "oras", command...)
	cmd.Dir = directory
	cmd.Stderr = os.Stderr
	out, err := cmd.Output()
	return strings.TrimSpace(string(out)), err
}
func Fetch(reference, output, username, token string) (*Descriptor, string, error) {
	if !referencePattern.MatchString(reference) {
		return nil, "", fmt.Errorf("require a digest-pinned TLS registry component reference")
	}
	if output == "" {
		return nil, "", fmt.Errorf("output required")
	}
	if _, err := os.Lstat(output); !os.IsNotExist(err) {
		return nil, "", fmt.Errorf("output must not exist")
	}
	if err := os.MkdirAll(filepath.Dir(output), 0755); err != nil {
		return nil, "", err
	}
	temporary, err := os.MkdirTemp(filepath.Dir(output), ".component-")
	if err != nil {
		return nil, "", err
	}
	defer os.RemoveAll(temporary)
	config, cleanup, err := auth(strings.Split(reference, "/")[0], username, token)
	if err != nil {
		return nil, "", err
	}
	defer cleanup()
	if _, err := oras("", config, "pull", reference, "--output", temporary); err != nil {
		return nil, "", err
	}
	metadata, digest, err := Verify(temporary)
	if err != nil {
		return nil, "", err
	}
	if _, err := os.Lstat(output); !os.IsNotExist(err) {
		return nil, "", fmt.Errorf("output appeared while downloading")
	}
	if err := os.Chmod(temporary, 0755); err != nil {
		return nil, "", err
	}
	if err := os.Rename(temporary, output); err != nil {
		return nil, "", err
	}
	return metadata, digest, nil
}
func Publish(directory, registry, username, token string) (string, string, error) {
	if !registryPattern.MatchString(registry) {
		return "", "", fmt.Errorf("expected a TLS registry repository without a tag or digest")
	}
	metadata, descriptor, err := Verify(directory)
	if err != nil {
		return "", "", err
	}
	if metadata.Source["dirty"] != false {
		return "", "", fmt.Errorf("publication requires clean build provenance")
	}
	if !regexp.MustCompile(`^[a-f0-9]{40}$`).MatchString(metadata.Source["commit"].(string)) {
		return "", "", fmt.Errorf("publication requires a Git commit identity")
	}
	config, cleanup, err := auth(strings.Split(registry, "/")[0], username, token)
	if err != nil {
		return "", "", err
	}
	defer cleanup()
	layout, err := os.MkdirTemp("", "oras-layout-")
	if err != nil {
		return "", "", err
	}
	defer os.RemoveAll(layout)
	files := []string{"component.json"}
	for name := range metadata.Files {
		files = append(files, name)
	}
	sort.Strings(files)
	args := []string{"push", "--oci-layout", layout + ":component", "--artifact-type", "application/vnd.pantavisor.component.v2", "--annotation", "org.opencontainers.image.created=1970-01-01T00:00:00Z"}
	args = append(args, "--")
	args = append(args, files...)
	if _, err := oras(directory, config, args...); err != nil {
		return "", "", err
	}
	_, after, err := Verify(directory)
	if err != nil || after != descriptor {
		return "", "", fmt.Errorf("component changed during publication staging")
	}
	digest, err := oras("", config, "resolve", "--oci-layout", layout+":component")
	if err != nil {
		return "", "", err
	}
	if !regexp.MustCompile(`^sha256:[a-f0-9]{64}$`).MatchString(digest) {
		return "", "", fmt.Errorf("invalid staged manifest digest")
	}
	reference := registry + "@" + digest
	// Publishing by digest makes retries idempotent and cannot overwrite a version
	// tag. The GitHub Release manifest supplies the version-to-digest mapping.
	if _, err := oras("", config, "cp", "--from-oci-layout", layout+"@"+digest, reference); err != nil {
		return "", "", err
	}
	resolved, err := oras("", config, "resolve", reference)
	if err != nil || resolved != digest {
		return "", "", fmt.Errorf("published manifest digest differs")
	}
	return reference, descriptor, nil
}
