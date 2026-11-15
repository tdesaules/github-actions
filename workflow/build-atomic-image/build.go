package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"syscall"
	"time"
)

const buildkitImage = "docker.io/moby/buildkit@sha256:aa632b921ef07411d2cb77292cd34213d4f7b384001d6fceef1e1f25bf9752bb"
const dockerSocket = "unix:///var/run/docker.sock"

type result struct {
	Archive, SHA256 string
	Size            int64
}
type command struct {
	Config, Workspace, Step string
	Args                    []string
	Timeout                 time.Duration
	Capture                 bool
}
type dockerRunner func(context.Context, command) ([]byte, error)

// Use a private client environment: no inherited registry credentials, contexts,
// SSH forwarding, API overrides, remote endpoints or Buildx builder selection.
func clientEnv(config string) []string {
	return []string{"PATH=/usr/local/bin:/usr/bin:/bin", "HOME=" + config,
		"DOCKER_CONFIG=" + config, "BUILDX_CONFIG=" + filepath.Join(config, "buildx")}
}

type boundedBuffer struct{ bytes.Buffer }

func (b *boundedBuffer) Write(p []byte) (int, error) {
	if b.Len()+len(p) > 1<<20 {
		return 0, errors.New("Docker response exceeds 1 MiB")
	}
	return b.Buffer.Write(p)
}
func executeDocker(parent context.Context, c command) ([]byte, error) {
	return executeClient(parent, c, "/usr/local/bin/docker")
}

// The executable parameter is internal to process-lifecycle fixtures, never an
// action input. Production always invokes the pinned absolute Docker CLI path.
func executeClient(parent context.Context, c command, executable string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(parent, c.Timeout)
	defer cancel()
	args := append([]string{"--config", c.Config, "--host", dockerSocket}, c.Args...)
	cmd := exec.CommandContext(ctx, executable, args...)
	cmd.Env = clientEnv(c.Config)
	cmd.Dir = c.Workspace
	cmd.Stderr = os.Stderr
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	// Buildx is a child process. Kill the whole client process group on
	// cancellation; the separately bounded builder removal stops the solve.
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
	cmd.WaitDelay = 5 * time.Second
	var output boundedBuffer
	if c.Capture {
		cmd.Stdout = &output
	} else {
		cmd.Stdout = os.Stderr
	}
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return nil, fmt.Errorf("%s cancelled or timed out: %w", c.Step, ctx.Err())
		}
		return nil, fmt.Errorf("%s failed: %w; see Docker/BuildKit diagnostics above", c.Step, err)
	}
	return output.Bytes(), nil
}

func relative(name string) error {
	if name == "" || filepath.IsAbs(name) || strings.ContainsAny(name, "\\,\x00\r\n") {
		return errors.New("paths must be workspace-relative without comma or control characters")
	}
	for _, part := range strings.Split(name, "/") {
		if part == ".." {
			return errors.New("parent traversal is forbidden")
		}
	}
	return nil
}
func within(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
func inputPath(workspace, name string, directory bool) (string, error) {
	if err := relative(name); err != nil {
		return "", err
	}
	path, err := filepath.EvalSymlinks(filepath.Join(workspace, name))
	if err != nil {
		return "", err
	}
	if !within(workspace, path) {
		return "", errors.New("input path escapes workspace")
	}
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	if directory && !info.IsDir() || !directory && !info.Mode().IsRegular() {
		return "", errors.New("context must be a directory and Containerfile a regular file")
	}
	return path, nil
}
func jsonArgs(raw, flagName string) ([]string, map[string]string, error) {
	if len(raw) > 1<<20 {
		return nil, nil, errors.New("JSON input exceeds 1 MiB")
	}
	var m map[string]string
	if err := json.Unmarshal([]byte(raw), &m); err != nil || m == nil {
		return nil, nil, fmt.Errorf("%s requires a JSON object of strings", flagName)
	}
	keys := make([]string, 0, len(m))
	for k, v := range m {
		if k == "" || strings.ContainsAny(k, "=\x00\r\n") || strings.ContainsRune(v, '\x00') {
			return nil, nil, fmt.Errorf("invalid %s key or value", flagName)
		}
		keys = append(keys, k)
	}
	sort.Strings(keys)
	args := []string{}
	for _, k := range keys {
		args = append(args, flagName, k+"="+m[k])
	}
	return args, m, nil
}
func validateIdentity(o options, arch string) error {
	if (arch != "amd64" && arch != "arm64") || o.Platform != "linux/"+arch {
		return errors.New("platform must be linux/amd64 or linux/arm64 matching the action's native architecture")
	}
	if !regexp.MustCompile(`^[a-z0-9]+(?:[._-][a-z0-9]+)*(?::[0-9]+)?(?:/[a-z0-9]+(?:[._-][a-z0-9]+)*)*$`).MatchString(o.Image) ||
		(strings.Contains(o.Image, ":") && !strings.Contains(o.Image, "/")) {
		return errors.New("image must be an untagged lowercase image name, optionally registry-qualified")
	}
	if !regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9_.-]{0,127}$`).MatchString(o.Version) {
		return errors.New("version must be a valid image tag")
	}
	if o.Target != "" && !regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]*$`).MatchString(o.Target) {
		return errors.New("invalid build target")
	}
	return nil
}
func serverPlatform(raw []byte, arch string) error {
	var s struct{ Arch, Os string }
	if err := json.Unmarshal(raw, &s); err != nil {
		return fmt.Errorf("cannot decode Docker server version: %w", err)
	}
	switch s.Arch {
	case "x86_64":
		s.Arch = "amd64"
	case "aarch64":
		s.Arch = "arm64"
	}
	if s.Os != "linux" || s.Arch != arch {
		return fmt.Errorf("Docker server platform %s/%s does not match native linux/%s", s.Os, s.Arch, arch)
	}
	return nil
}

func build(ctx context.Context, workspace, arch string, o options, run dockerRunner) (r result, err error) {
	if err = validateIdentity(o, arch); err != nil {
		return r, err
	}
	workspace, err = filepath.Abs(workspace)
	if err != nil {
		return r, err
	}
	workspace, err = filepath.EvalSymlinks(workspace)
	if err != nil {
		return r, err
	}
	if strings.ContainsAny(workspace, ",\x00\r\n") {
		return r, errors.New("workspace path cannot contain OCI exporter separators or control characters")
	}
	contextPath, err := inputPath(workspace, o.Context, true)
	if err != nil {
		return r, fmt.Errorf("context: %w", err)
	}
	containerfile, err := inputPath(workspace, o.Containerfile, false)
	if err != nil {
		return r, fmt.Errorf("containerfile: %w", err)
	}
	if err = relative(o.Output); err != nil {
		return r, fmt.Errorf("output: %w", err)
	}
	o.Output = filepath.Clean(o.Output)
	if o.Output == "." {
		return r, errors.New("output must name a new file")
	}
	labels, wantedLabels, err := jsonArgs(o.Labels, "--label")
	if err != nil {
		return r, err
	}
	buildArgs, _, err := jsonArgs(o.BuildArgs, "--build-arg")
	if err != nil {
		return r, err
	}
	root, err := os.OpenRoot(workspace)
	if err != nil {
		return r, err
	}
	defer root.Close()
	if _, err = root.Lstat(o.Output); !os.IsNotExist(err) {
		return r, errors.New("output already exists or is inaccessible; choose a new archive path")
	}
	// Root confines every output filesystem operation, including symlink races.
	if err = root.MkdirAll(filepath.Dir(o.Output), 0755); err != nil {
		return r, fmt.Errorf("output directory: %w", err)
	}
	parent, err := filepath.EvalSymlinks(filepath.Join(workspace, filepath.Dir(o.Output)))
	if err != nil {
		return r, err
	}
	if !within(workspace, parent) {
		return r, errors.New("output directory escapes workspace")
	}
	var random [16]byte
	if _, err = rand.Read(random[:]); err != nil {
		return r, err
	}
	name := "atomic-" + hex.EncodeToString(random[:])
	stage := filepath.Join(filepath.Dir(o.Output), "."+name)
	if err = root.Mkdir(stage, 0700); err != nil {
		return r, err
	}
	defer func() { err = errors.Join(err, root.RemoveAll(stage)) }()
	// Client configuration lives outside the uploaded context.
	config, err := os.MkdirTemp("/tmp", "atomic-docker-")
	if err != nil {
		return r, err
	}
	defer func() { err = errors.Join(err, os.RemoveAll(config)) }()
	if within(contextPath, config) {
		return r, errors.New("private Docker configuration must be outside the uploaded context")
	}
	invoke := func(c context.Context, step string, timeout time.Duration, capture bool, args ...string) ([]byte, error) {
		return run(c, command{Config: config, Workspace: workspace, Step: step, Timeout: timeout, Capture: capture, Args: args})
	}
	staged := filepath.Join(stage, "image.oci.tar")
	err = func() (buildErr error) {
		server, e := invoke(ctx, "Docker server discovery (requires the runner Docker socket)", 30*time.Second, true, "version", "--format", "{{json .Server}}")
		if e != nil {
			return e
		}
		if e = serverPlatform(server, arch); e != nil {
			return e
		}
		if _, e = invoke(ctx, "create private Buildx builder", time.Minute, true, "buildx", "create", "--name", name,
			"--driver", "docker-container", "--driver-opt", "image="+buildkitImage,
			"--buildkitd-flags=--oci-worker-gc=true"); e != nil {
			return e
		}
		// No daemon state exists until bootstrap. Register cleanup immediately
		// after successful creation, including bootstrap/build/cancellation errors.
		defer func() {
			cleanupCtx, cancel := context.WithTimeout(context.Background(), time.Minute)
			defer cancel()
			if _, e := invoke(cleanupCtx, "remove owned Buildx builder "+name, time.Minute, false, "buildx", "rm", "--force", name); e != nil {
				buildErr = errors.Join(buildErr, fmt.Errorf("cleanup failed for builder %s (only its container/state may need removal): %w", name, e))
			}
		}()
		if _, e = invoke(ctx, "bootstrap pinned BuildKit", 5*time.Minute, false, "buildx", "inspect", "--bootstrap", name); e != nil {
			return e
		}
		args := []string{"buildx", "build", "--builder", name, "--platform", o.Platform, "--file", containerfile,
			"--tag", o.Image + ":" + o.Version, "--progress=plain", "--provenance=false", "--sbom=false",
			"--output", "type=oci,dest=" + filepath.Join(workspace, staged) + ",oci-mediatypes=true"}
		args = append(args, labels...)
		args = append(args, buildArgs...)
		if o.Target != "" {
			args = append(args, "--target", o.Target)
		}
		args = append(args, "--", contextPath)
		if _, e = invoke(ctx, "build native Atomic OCI archive", 2*time.Hour, false, args...); e != nil {
			return e
		}
		info, e := root.Lstat(staged)
		if e != nil {
			return e
		}
		if !info.Mode().IsRegular() {
			return errors.New("BuildKit did not export a regular OCI archive")
		}
		f, e := root.Open(staged)
		if e != nil {
			return e
		}
		r.SHA256, r.Size, e = verifyArchive(ctx, f, arch, wantedLabels)
		if e == nil {
			// The native Docker action may run as root while artifact transfer
			// runs as the host runner user. Permissions do not change tar bytes.
			e = f.Chmod(0644)
		}
		return errors.Join(e, f.Close())
	}()
	if err != nil {
		return r, err
	}
	if err = ctx.Err(); err != nil {
		return r, err
	}
	// Hard-link creation is exclusive and preserves the exact verified bytes.
	if err = root.Link(staged, o.Output); err != nil {
		return r, fmt.Errorf("publish archive without overwriting output: %w", err)
	}
	r.Archive = o.Output
	return r, nil
}

var _ io.Writer = (*boundedBuffer)(nil)
