package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/tdesaules/github-actions/shared/workflow"
)

func hash(path string) (string, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("expected a regular file: %s", path)
	}
	f, err := os.Open(path)
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

func workspacePath(name string, input bool) (string, error) {
	if name == "" || strings.ContainsAny(name, "\x00\r\n") {
		return "", fmt.Errorf("workspace paths must not be empty or contain control characters")
	}
	root, err := os.Getwd()
	if err != nil {
		return "", err
	}
	path, err := filepath.Abs(name)
	if err != nil {
		return "", err
	}
	within := func(candidate string) bool {
		relative, err := filepath.Rel(root, candidate)
		return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
	}
	if !within(path) {
		return "", fmt.Errorf("path must remain in the workspace")
	}
	if input {
		info, err := os.Lstat(path)
		if err != nil {
			return "", err
		}
		if !info.Mode().IsRegular() {
			return "", fmt.Errorf("source archive must be a regular file, not a device or symlink")
		}
	}
	parent := filepath.Dir(path)
	for {
		resolved, err := filepath.EvalSymlinks(parent)
		if err == nil {
			if !within(resolved) {
				return "", fmt.Errorf("parent directory escapes workspace")
			}
			break
		}
		if !os.IsNotExist(err) || parent == root {
			return "", err
		}
		parent = filepath.Dir(parent)
	}
	return path, nil
}

func validate(repository, expected string) error {
	if !regexp.MustCompile(`^ghcr\.io/[a-z0-9][a-z0-9._-]*/[a-z0-9][a-z0-9._-]*$`).MatchString(repository) {
		return fmt.Errorf("expected a GHCR owner/image repository")
	}
	if !regexp.MustCompile(`^[a-f0-9]{64}$`).MatchString(expected) {
		return fmt.Errorf("expected archive SHA-256")
	}
	return nil
}

func runCommand(ctx context.Context, directory, name string, args ...string) error {
	cmd := process(ctx, name, args...)
	cmd.Dir = directory
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	if name == "make" {
		// The upstream Makefile contains a misspelled GITUHB_OUTPUT redirect.
		// This entrypoint owns the action outputs after complete validation.
		// Cmd.Environ derives PWD from Dir; the upstream Makefile uses $(PWD)
		// for template/repository paths rather than Make's built-in CURDIR.
		for _, value := range cmd.Environ() {
			if !strings.HasPrefix(value, "GITHUB_OUTPUT=") {
				cmd.Env = append(cmd.Env, value)
			}
		}
	}
	return cmd.Run()
}

func run() error {
	archive := flag.String("archive", "", "native OCI archive to embed")
	expected := flag.String("sha256", "", "archive checksum from the builder")
	repository := flag.String("repository", "", "OS registry/owner/image")
	revision := flag.String("revision", "", "source commit")
	version := flag.String("version", "", "candidate version")
	output := flag.String("output", ".build/niri-tomic-os-x86_64.iso", "ISO output")
	flag.Parse()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	ctx, cancel := context.WithTimeout(ctx, 2*time.Hour)
	defer cancel()
	if err := runIdentity(os.Getenv("GITHUB_RUN_ID"), os.Getenv("GITHUB_RUN_ATTEMPT")); err != nil {
		return err
	}
	if flag.NArg() != 0 || *archive == "" || *output == "" {
		return fmt.Errorf("archive and output are required")
	}
	if err := validate(*repository, *expected); err != nil {
		return err
	}
	if !regexp.MustCompile(`^[a-f0-9]{40}$`).MatchString(*revision) ||
		!regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]*$`).MatchString(*version) {
		return fmt.Errorf("source revision and candidate version required")
	}
	if runtime.GOARCH != "amd64" {
		return fmt.Errorf("installer output currently supports native AMD64 only")
	}
	archivePath, err := workspacePath(*archive, true)
	if err != nil {
		return err
	}
	sum, err := hash(archivePath)
	if err != nil {
		return err
	}
	if sum != *expected {
		return fmt.Errorf("OS archive checksum mismatch")
	}
	configCommand := process(ctx, "skopeo", "inspect", "--config", "oci-archive:"+archivePath)
	configCommand.Stderr = os.Stderr
	configBytes, err := configCommand.Output()
	if err != nil {
		return err
	}
	var config struct {
		OS           string `json:"os"`
		Architecture string `json:"architecture"`
		Config       struct {
			Labels map[string]string `json:"Labels"`
		} `json:"config"`
	}
	if err := json.Unmarshal(configBytes, &config); err != nil {
		return err
	}
	if config.OS != "linux" || config.Architecture != "amd64" || config.Config.Labels["org.opencontainers.image.revision"] != *revision {
		return fmt.Errorf("OS archive platform or source revision mismatch")
	}
	outputPath, err := workspacePath(*output, false)
	if err != nil {
		return err
	}
	for _, path := range []string{outputPath, outputPath + "-CHECKSUM", outputPath + ".build.json"} {
		if _, err := os.Lstat(path); !os.IsNotExist(err) {
			return fmt.Errorf("ISO output and sidecars must not already exist")
		}
	}
	if os.Getenv("ATOMIC_LORAX_WORKER") != "1" {
		return launchWorker(ctx, os.Args[1:])
	}
	work, err := os.MkdirTemp("", "atomic-iso-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(work)
	if err := runCommand(ctx, "", "cp", "-a", "/usr/share/atomic-installer/.", work); err != nil {
		return err
	}
	// Canonical internal names avoid Make/xorriso interpreting caller paths.
	if err := os.Symlink(archivePath, filepath.Join(work, "source.oci.tar")); err != nil {
		return err
	}
	if _, err := os.Stat("/dev/loop0"); os.IsNotExist(err) {
		if err := runCommand(ctx, "", "mknod", "-m", "0660", "/dev/loop0", "b", "7", "0"); err != nil {
			return err
		}
	}
	parts := strings.Split(*repository, "/")
	iso := filepath.Join(work, "image.iso")
	args := []string{
		"ARCH=x86_64", "VERSION=45", "VARIANT=Server", "ROOTFS_SIZE=4",
		"WEB_UI=false", "IMAGE_SIGNED=true", "IMAGE_NAME=" + parts[2],
		"IMAGE_REPO=" + strings.Join(parts[:2], "/"), "IMAGE_TAG=45",
		"IMAGE_SRC=oci-archive:" + filepath.Join(work, "source.oci.tar"),
		"ISO_NAME=" + iso,
	}
	if err := runCommand(ctx, work, "make", args...); err != nil {
		return fmt.Errorf("Lorax ISO build: %w", err)
	}
	if err := runCommand(ctx, "", "checkisomd5", "--verbose", iso); err != nil {
		return fmt.Errorf("generated ISO media checksum: %w", err)
	}
	after, err := hash(archivePath)
	if err != nil || after != sum {
		return fmt.Errorf("OS archive changed during ISO construction: %v", err)
	}
	isoHash, err := hash(iso)
	if err != nil {
		return err
	}
	root, err := os.Getwd()
	if err != nil {
		return err
	}
	*output, err = filepath.Rel(root, outputPath)
	if err != nil {
		return err
	}
	info, err := os.Stat(iso)
	if err != nil {
		return err
	}
	size := info.Size()
	receipt := map[string]any{
		"schema_version": 1, "component": "iso", "arch": "amd64",
		"source_sha": *revision, "version": *version, "archive": filepath.Base(*output),
		"sha256": isoHash, "size": size, "os_archive_sha256": sum, "tested": false,
		"run_id": os.Getenv("GITHUB_RUN_ID"), "run_attempt": os.Getenv("GITHUB_RUN_ATTEMPT"),
	}
	data, err := json.MarshalIndent(receipt, "", "  ")
	if err != nil {
		return err
	}
	if err := publishISO(ctx, iso, *output, isoHash, append(data, '\n')); err != nil {
		return err
	}
	for key, value := range map[string]any{
		"iso": *output, "sha256": isoHash, "size": size, "os_archive_sha256": sum,
		"receipt": *output + ".build.json",
	} {
		if err := workflow.Output(key, value); err != nil {
			return err
		}
	}
	return nil
}

func main() { workflow.Main("build-atomic-iso Fedora 45 / Lorax 45.4", run) }
