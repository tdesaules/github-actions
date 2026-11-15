package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func fixture(t *testing.T) (string, options) {
	t.Helper()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "Containerfile"), []byte("FROM scratch\n"), 0644); err != nil {
		t.Fatal(err)
	}
	o, err := parse([]string{"--image=ghcr.io/owner/os", "--version=1.0.0", "--platform=linux/amd64", "--output=.build/image.oci.tar"})
	if err != nil {
		t.Fatal(err)
	}
	return root, o
}
func argument(args []string, key string) string {
	for i, v := range args {
		if v == key && i+1 < len(args) {
			return args[i+1]
		}
	}
	return ""
}

type dockerMock struct {
	t       *testing.T
	calls   []command
	archive []byte
	fail    string
	server  string
	onBuild func()
}

func (m *dockerMock) run(ctx context.Context, c command) ([]byte, error) {
	m.t.Helper()
	m.calls = append(m.calls, c)
	if c.Config == "" || c.Timeout <= 0 || c.Timeout > 2*time.Hour {
		m.t.Fatal("missing private configuration or timeout")
	}
	if info, err := os.Stat(c.Config); err != nil || info.Mode().Perm() != 0700 {
		m.t.Fatal("client config is not private")
	}
	if within(c.Workspace, c.Config) {
		m.t.Fatal("client configuration is inside the workspace")
	}
	if c.Args[0] == "version" {
		if m.server != "" {
			return []byte(m.server), nil
		}
		return []byte(`{"Arch":"amd64","Os":"linux","Version":"28.0.4"}`), nil
	}
	op := c.Args[1]
	if op == "rm" && ctx.Err() != nil {
		m.t.Fatal("cleanup inherited cancelled build context")
	}
	if op == m.fail {
		return nil, errors.New("injected " + op + " failure")
	}
	if op == "build" {
		dest := strings.TrimSuffix(strings.TrimPrefix(argument(c.Args, "--output"), "type=oci,dest="), ",oci-mediatypes=true")
		if err := os.WriteFile(dest, m.archive, 0600); err != nil {
			m.t.Fatal(err)
		}
		if m.onBuild != nil {
			m.onBuild()
		}
	}
	return nil, nil
}
func assertPrivateCleanup(t *testing.T, root string, m *dockerMock) {
	t.Helper()
	for _, c := range m.calls {
		if _, err := os.Stat(c.Config); !os.IsNotExist(err) {
			t.Fatal("private client configuration leaked")
		}
	}
	matches, err := filepath.Glob(filepath.Join(root, ".build", ".atomic-*"))
	if err != nil || len(matches) != 0 {
		t.Fatal("private staging directory leaked")
	}
}

func TestFixedBuildExportsExactArchiveAndRemovesOwnedBuilder(t *testing.T) {
	root, o := fixture(t)
	t.Setenv("TMPDIR", root)
	o.Labels = `{"z":"$(touch /outside)","a":"two words"}`
	o.BuildArgs = `{"FEDORA_VERSION":"45"}`
	o.Target = "final"
	data := archiveFixture(t, "amd64", map[string]string{"z": "$(touch /outside)", "a": "two words"}, "")
	m := &dockerMock{t: t, archive: data}
	r, err := build(context.Background(), root, "amd64", o, m.run)
	if err != nil {
		t.Fatal(err)
	}
	if r.Archive != o.Output || r.Size != int64(len(data)) || r.SHA256 != fmt.Sprintf("%x", sha256.Sum256(data)) {
		t.Fatal("wrong archive outputs")
	}
	native, config := fixtureManifestDigests(t, data)
	if r.ManifestDigest != native || r.ManifestDigest == config {
		t.Fatal("build returned incorrect manifest identity")
	}
	outputFile := filepath.Join(root, "github-output")
	if err := os.WriteFile(outputFile, nil, 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GITHUB_OUTPUT", outputFile)
	if err := outputs(r); err != nil {
		t.Fatal(err)
	}
	outputData, err := os.ReadFile(outputFile)
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range []string{"archive=" + o.Output, "archive-sha256=" + r.SHA256, "manifest-digest=" + native, fmt.Sprintf("size=%d", len(data))} {
		if !strings.Contains("\n"+string(outputData), "\n"+line+"\n") {
			t.Fatalf("missing action output %s", line)
		}
	}
	got, err := os.ReadFile(filepath.Join(root, o.Output))
	if err != nil || !bytes.Equal(got, data) {
		t.Fatal("export bytes changed")
	}
	if info, err := os.Stat(filepath.Join(root, o.Output)); err != nil || info.Mode().Perm() != 0644 {
		t.Fatal("archive is not readable by host artifact-transfer steps")
	}
	source, err := os.ReadFile(filepath.Join(root, "Containerfile"))
	if err != nil || string(source) != "FROM scratch\n" {
		t.Fatal("Containerfile was modified")
	}
	if len(m.calls) != 5 {
		t.Fatalf("unexpected operations: %+v", m.calls)
	}
	create := m.calls[1].Args
	name := argument(create, "--name")
	if !strings.HasPrefix(name, "atomic-") || len(name) != 39 || argument(create, "--driver") != "docker-container" || argument(create, "--driver-opt") != "image="+buildkitImage {
		t.Fatal("builder is not uniquely named and pinned")
	}
	args := m.calls[3].Args
	for _, required := range []string{"--provenance=false", "--sbom=false", "--progress=plain", "z=$(touch /outside)", "a=two words", "FEDORA_VERSION=45"} {
		found := false
		for _, arg := range args {
			if arg == required {
				found = true
			}
		}
		if !found {
			t.Fatalf("missing literal argument %q", required)
		}
	}
	if argument(args, "--builder") != name || argument(args, "--platform") != "linux/amd64" || argument(args, "--target") != "final" {
		t.Fatal("incorrect native build arguments")
	}
	for _, c := range m.calls {
		for _, arg := range c.Args {
			if arg == "--push" || arg == "--load" || arg == "prune" || arg == "--use" {
				t.Fatal("unapproved engine operation")
			}
		}
	}
	if !reflect.DeepEqual(m.calls[4].Args, []string{"buildx", "rm", "--force", name}) {
		t.Fatal("cleanup is not scoped to owned builder")
	}
	assertPrivateCleanup(t, root, m)
}

func TestFailuresAndCancellationCleanOnlyOwnedResources(t *testing.T) {
	for _, fail := range []string{"create", "inspect", "build", "rm", "cancel", "invalid-archive"} {
		t.Run(fail, func(t *testing.T) {
			root, o := fixture(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			m := &dockerMock{t: t, archive: archiveFixture(t, "amd64", nil, ""), fail: fail}
			if fail == "cancel" {
				m.onBuild = cancel
			}
			if fail == "invalid-archive" {
				m.archive = []byte("invalid")
			}
			if _, err := build(ctx, root, "amd64", o, m.run); err == nil {
				t.Fatal("failure was not blocking")
			}
			if _, err := os.Lstat(filepath.Join(root, o.Output)); !os.IsNotExist(err) {
				t.Fatal("failed operation published an archive")
			}
			wantCleanup := fail != "create"
			gotCleanup := len(m.calls) > 0 && len(m.calls[len(m.calls)-1].Args) > 1 && m.calls[len(m.calls)-1].Args[1] == "rm"
			if wantCleanup != gotCleanup {
				t.Fatal("incorrect builder cleanup")
			}
			assertPrivateCleanup(t, root, m)
		})
	}
}

func TestOutputCollisionPreservesConcurrentWriter(t *testing.T) {
	root, o := fixture(t)
	m := &dockerMock{t: t, archive: archiveFixture(t, "amd64", nil, ""), onBuild: func() {
		if err := os.WriteFile(filepath.Join(root, o.Output), []byte("other writer"), 0600); err != nil {
			t.Fatal(err)
		}
	}}
	if _, err := build(context.Background(), root, "amd64", o, m.run); err == nil {
		t.Fatal("output collision accepted")
	}
	data, err := os.ReadFile(filepath.Join(root, o.Output))
	if err != nil || string(data) != "other writer" {
		t.Fatal("concurrent output was overwritten")
	}
	assertPrivateCleanup(t, root, m)
}

func TestRejectUnsafeInputsBeforeDocker(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(string, *options)
	}{
		{"parent-context", func(_ string, o *options) { o.Context = "../" }},
		{"absolute-context", func(_ string, o *options) { o.Context = "/tmp" }},
		{"parent-file", func(_ string, o *options) { o.Containerfile = "../Containerfile" }},
		{"parent-output", func(_ string, o *options) { o.Output = "../out" }},
		{"csv-output", func(_ string, o *options) { o.Output = "out,push=true" }},
		{"existing-output", func(_ string, o *options) { o.Output = "Containerfile" }},
		{"cross-arch", func(_ string, o *options) { o.Platform = "linux/arm64" }},
		{"multiplatform", func(_ string, o *options) { o.Platform = "linux/amd64,linux/arm64" }},
		{"tagged-image", func(_ string, o *options) { o.Image = "owner/os:tag" }},
		{"bad-version", func(_ string, o *options) { o.Version = "1.0.0\npush=true" }},
		{"bad-labels", func(_ string, o *options) { o.Labels = `{"foo":4}` }},
		{"null-labels", func(_ string, o *options) { o.Labels = `null` }},
		{"escaping-context-link", func(root string, o *options) {
			if err := os.Symlink(t.TempDir(), filepath.Join(root, "escape")); err != nil {
				t.Fatal(err)
			}
			o.Context = "escape"
		}},
		{"escaping-file-link", func(root string, o *options) {
			if err := os.Symlink("/etc/passwd", filepath.Join(root, "escape")); err != nil {
				t.Fatal(err)
			}
			o.Containerfile = "escape"
		}},
		{"escaping-output-link", func(root string, o *options) {
			if err := os.Symlink(t.TempDir(), filepath.Join(root, "escape")); err != nil {
				t.Fatal(err)
			}
			o.Output = "escape/archive.tar"
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root, o := fixture(t)
			tc.change(root, &o)
			called := false
			_, err := build(context.Background(), root, "amd64", o, func(context.Context, command) ([]byte, error) { called = true; return nil, nil })
			if err == nil || called {
				t.Fatalf("unsafe input reached Docker: %v", err)
			}
		})
	}
}

func TestServerArchitectureMustMatchNativeAction(t *testing.T) {
	for _, server := range []string{`{"Arch":"arm64","Os":"linux"}`, `{"Arch":"amd64","Os":"windows"}`, `null`, `invalid`} {
		root, o := fixture(t)
		m := &dockerMock{t: t, server: server}
		if _, err := build(context.Background(), root, "amd64", o, m.run); err == nil {
			t.Fatal("incompatible server accepted")
		}
		if len(m.calls) != 1 {
			t.Fatal("builder created on incompatible server")
		}
		assertPrivateCleanup(t, root, m)
	}
	root, o := fixture(t)
	o.Platform = "linux/arm64"
	m := &dockerMock{t: t, server: `{"Arch":"aarch64","Os":"linux"}`, archive: archiveFixture(t, "arm64", nil, "")}
	if _, err := build(context.Background(), root, "arm64", o, m.run); err != nil {
		t.Fatal(err)
	}
}

func TestPrivateEnvironmentAndFixedCLI(t *testing.T) {
	t.Setenv("DOCKER_HOST", "tcp://unrelated:2375")
	t.Setenv("DOCKER_AUTH_CONFIG", "credentials")
	t.Setenv("DOCKER_API_VERSION", "1.99")
	env := strings.Join(clientEnv("/private"), "\n")
	if strings.Contains(env, "unrelated") || strings.Contains(env, "credentials") || strings.Contains(env, "1.99") {
		t.Fatal("ambient Docker configuration inherited")
	}
	for _, args := range [][]string{{"--operation=push"}, {"--secrets=[]"}, {"--builder-image=other"}, {"build"}, {"--registry-password=secret"}} {
		if _, err := parse(args); err == nil {
			t.Fatal("arbitrary operations/credentials accepted")
		}
	}
}

func TestClientCancellationKillsPluginProcessGroup(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "client")
	// A live child keeps the captured output pipe open if only its parent dies.
	if err := os.WriteFile(script, []byte("#!/bin/sh\nsleep 60 &\nwait\n"), 0700); err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	_, err := executeClient(context.Background(), command{Config: dir, Workspace: dir, Step: "fixture", Timeout: 500 * time.Millisecond, Capture: true}, script)
	if err == nil || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected bounded cancellation, got %v", err)
	}
	if time.Since(start) > 4*time.Second {
		t.Fatal("client/plugin process group did not terminate promptly")
	}
}

func TestClientUsesOnlyFixedSocketAndPrivateEnvironment(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "client")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nprintf '%s\\n' \"$@\" \"CONFIG=$DOCKER_CONFIG\" \"HOST=$DOCKER_HOST\" \"AUTH=$DOCKER_AUTH_CONFIG\"\n"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DOCKER_HOST", "tcp://unrelated:2375")
	t.Setenv("DOCKER_AUTH_CONFIG", "sensitive")
	data, err := executeClient(context.Background(), command{Config: dir, Workspace: dir, Step: "fixture", Timeout: time.Second, Capture: true, Args: []string{"version", "--format", "{{json .Server}}"}}, script)
	if err != nil {
		t.Fatal(err)
	}
	want := strings.Join([]string{"--config", dir, "--host", dockerSocket, "version", "--format", "{{json .Server}}", "CONFIG=" + dir, "HOST=", "AUTH=", ""}, "\n")
	if string(data) != want {
		t.Fatalf("unexpected private client invocation: %s", data)
	}
}
