package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"time"
)

type actionContainer struct {
	ID     string `json:"id"`
	Image  string `json:"image"`
	Mounts []struct {
		Type        string `json:"Type"`
		Source      string `json:"Source"`
		Destination string `json:"Destination"`
		RW          bool   `json:"RW"`
	} `json:"mounts"`
}

func workerMounts(container actionContainer, hostname, output string) (string, string, error) {
	if !regexp.MustCompile(`^[a-f0-9]{12,64}$`).MatchString(hostname) ||
		!regexp.MustCompile(`^[a-f0-9]{64}$`).MatchString(container.ID) ||
		!strings.HasPrefix(container.ID, hostname) ||
		!regexp.MustCompile(`^sha256:[a-f0-9]{64}$`).MatchString(container.Image) {
		return "", "", fmt.Errorf("worker must use the current action's immutable local image")
	}
	if output != "/github/output" &&
		(filepath.Dir(output) != "/github/file_commands" || !regexp.MustCompile(`^set_output_[A-Za-z0-9_-]+$`).MatchString(filepath.Base(output))) {
		return "", "", fmt.Errorf("expected the action's GitHub output command file")
	}
	workspace, outputSource := "", ""
	for _, mount := range container.Mounts {
		if strings.HasPrefix(mount.Destination, "/github/workspace/") {
			return "", "", fmt.Errorf("nested workspace mounts cannot be reproduced safely")
		}
		if mount.Destination != "/github/workspace" && mount.Destination != "/github/file_commands" && mount.Destination != "/github/output" {
			continue
		}
		if mount.Type != "bind" || !mount.RW {
			continue
		}
		if !filepath.IsAbs(mount.Source) || mount.Source == "/" ||
			filepath.Clean(mount.Source) != mount.Source || strings.ContainsAny(mount.Source, ",\x00\r\n") {
			return "", "", fmt.Errorf("unsupported host bind source")
		}
		switch mount.Destination {
		case "/github/workspace":
			if workspace != "" {
				return "", "", fmt.Errorf("ambiguous workspace mount")
			}
			workspace = mount.Source
		case "/github/file_commands":
			if filepath.Dir(output) == mount.Destination {
				if outputSource != "" {
					return "", "", fmt.Errorf("ambiguous output mount")
				}
				outputSource = filepath.Join(mount.Source, filepath.Base(output))
			}
		case "/github/output":
			if output == mount.Destination {
				if outputSource != "" {
					return "", "", fmt.Errorf("ambiguous output mount")
				}
				outputSource = mount.Source
			}
		}
	}
	if workspace == "" || outputSource == "" {
		return "", "", fmt.Errorf("workspace and output bind mounts are required")
	}
	return workspace, outputSource, nil
}

// launchWorker implements the user-approved Lorax-only privilege exception.
// The worker gets the same immutable image, no engine socket and no credentials.
func launchWorker(ctx context.Context, arguments []string) error {
	info, err := os.Stat("/var/run/docker.sock")
	if err != nil || info.Mode()&os.ModeSocket == 0 {
		return fmt.Errorf("Lorax requires the runner Docker socket or an explicitly privileged worker")
	}
	hostname, err := os.Hostname()
	if err != nil {
		return err
	}
	if info, err := os.Lstat(os.Getenv("GITHUB_OUTPUT")); err != nil || !info.Mode().IsRegular() {
		return fmt.Errorf("GitHub output command destination must be a regular file")
	}
	private, err := os.MkdirTemp("/tmp", "lorax-client-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(private)
	env := []string{"PATH=/usr/bin:/bin", "HOME=" + private, "DOCKER_CONFIG=" + private, "DOCKER_HOST=unix:///var/run/docker.sock"}
	// Inspect only identity/mount fields, never Config.Env, which may hold secrets.
	runDocker := func(parent context.Context, args []string, capture bool) ([]byte, error) {
		cmd := process(parent, "/usr/bin/docker", append([]string{"--host=unix:///var/run/docker.sock"}, args...)...)
		cmd.Env, cmd.Stderr = env, os.Stderr
		var data boundedOutput
		if capture {
			cmd.Stdout = &data
			cmd.Stderr = &data
		} else {
			cmd.Stdout = os.Stdout
		}
		err := cmd.Run()
		return data.Bytes(), err
	}
	inspectCtx, stop := context.WithTimeout(ctx, 30*time.Second)
	data, err := runDocker(inspectCtx, []string{"inspect", "--type", "container", "--format",
		`{"id":{{json .Id}},"image":{{json .Image}},"mounts":{{json .Mounts}}}`, hostname}, true)
	stop()
	if err != nil {
		return fmt.Errorf("cannot inspect current action container")
	}
	var container actionContainer
	if err := json.Unmarshal(data, &container); err != nil {
		return fmt.Errorf("invalid current-container descriptor")
	}
	return coordinate(ctx, container, hostname, os.Getenv("GITHUB_OUTPUT"), os.Getenv("GITHUB_RUN_ID"), os.Getenv("GITHUB_RUN_ATTEMPT"), arguments, runDocker)
}

type boundedOutput struct{ bytes.Buffer }

func (b *boundedOutput) Write(p []byte) (int, error) {
	if b.Len()+len(p) > 1<<20 {
		return 0, fmt.Errorf("Docker response exceeds 1 MiB")
	}
	return b.Buffer.Write(p)
}
func process(ctx context.Context, name string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
	cmd.WaitDelay = 5 * time.Second
	return cmd
}
func runIdentity(run, attempt string) error {
	numeric := regexp.MustCompile(`^[1-9][0-9]*$`)
	if !numeric.MatchString(run) || !numeric.MatchString(attempt) {
		return fmt.Errorf("positive GitHub run ID and attempt required")
	}
	return nil
}

func coordinate(ctx context.Context, container actionContainer, hostname, commandFile, runID, attempt string, arguments []string, runDocker func(context.Context, []string, bool) ([]byte, error)) error {
	if err := runIdentity(runID, attempt); err != nil {
		return err
	}
	workspace, output, err := workerMounts(container, hostname, commandFile)
	if err != nil {
		return err
	}
	nonce := make([]byte, 16)
	if _, err := rand.Read(nonce); err != nil {
		return err
	}
	name := "atomic-lorax-" + hex.EncodeToString(nonce)
	ctx, timeout := context.WithTimeout(ctx, 2*time.Hour)
	defer timeout()
	args := []string{"run", "--rm", "--pull=never", "--privileged", "--name", name,
		"--entrypoint", "/entrypoint", "--workdir", "/github/workspace",
		"--mount", "type=bind,src=" + workspace + ",dst=/github/workspace",
		"--mount", "type=bind,src=" + output + ",dst=/github/output",
		"--env", "ATOMIC_LORAX_WORKER=1", "--env", "GITHUB_OUTPUT=/github/output",
		"--env", "GITHUB_RUN_ID=" + runID,
		"--env", "GITHUB_RUN_ATTEMPT=" + attempt, container.Image}
	_, runErr := runDocker(ctx, append(args, arguments...), false)
	cleanupContext, stop := context.WithTimeout(context.Background(), 15*time.Second)
	defer stop()
	cleanupOutput, cleanupErr := runDocker(cleanupContext, []string{"rm", "--force", name}, true)
	if cleanupErr != nil && !strings.Contains(string(cleanupOutput), "No such container") {
		cleanupErr = fmt.Errorf("owned Lorax worker %s cleanup failed: %w", name, cleanupErr)
	} else {
		cleanupErr = nil
	}
	if runErr != nil {
		runErr = fmt.Errorf("Lorax worker failed: %w", runErr)
	}
	return errors.Join(runErr, cleanupErr)
}
