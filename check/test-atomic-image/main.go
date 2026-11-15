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
	"runtime"
	"strings"

	"github.com/tdesaules/github-actions/shared/workflow"
)

var hashPattern = regexp.MustCompile(`^[a-f0-9]{64}$`)
var revisionPattern = regexp.MustCompile(`^[a-f0-9]{40}$`)

func hash(path string) (string, error) {
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

func command(name string, args ...string) ([]byte, error) {
	cmd := exec.Command(name, args...)
	cmd.Stderr = os.Stderr
	return cmd.Output()
}

func verifyConfig(data []byte, arch, revision string) error {
	var config struct {
		Architecture string `json:"architecture"`
		OS           string `json:"os"`
		Config       struct {
			Labels map[string]string `json:"Labels"`
		} `json:"config"`
	}
	if err := json.Unmarshal(data, &config); err != nil {
		return err
	}
	if config.Architecture != arch || config.OS != "linux" {
		return fmt.Errorf("unexpected OCI architecture or operating system")
	}
	if config.Config.Labels["org.opencontainers.image.revision"] != revision {
		return fmt.Errorf("source revision mismatch")
	}
	return nil
}

func run() error {
	archive := flag.String("archive", "", "exact built OCI archive")
	expected := flag.String("sha256", "", "expected archive checksum")
	revision := flag.String("revision", "", "expected source revision")
	arch := flag.String("architecture", "", "native architecture")
	version := flag.String("fedora-version", "45", "expected Fedora version")
	output := flag.String("output", "qualification.json", "qualification receipt")
	flag.Parse()
	if flag.NArg() != 0 || *archive == "" || !hashPattern.MatchString(*expected) ||
		!revisionPattern.MatchString(*revision) || (*arch != "amd64" && *arch != "arm64") ||
		*arch != runtime.GOARCH || !regexp.MustCompile(`^[0-9]+$`).MatchString(*version) {
		return fmt.Errorf("archive, checksum, source revision and matching native architecture required")
	}
	archivePath, err := filepath.Abs(*archive)
	if err != nil {
		return err
	}
	outputPath, err := filepath.Abs(*output)
	if err != nil {
		return err
	}
	if archivePath == outputPath {
		return fmt.Errorf("receipt must not overwrite archive")
	}
	before, err := hash(archivePath)
	if err != nil {
		return err
	}
	if before != *expected {
		return fmt.Errorf("archive checksum mismatch")
	}
	tmp, err := os.MkdirTemp("", "atomic-qualification-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	source := "oci-archive:" + archivePath
	config, err := command("skopeo", "inspect", "--config", source)
	if err != nil {
		return err
	}
	if err := verifyConfig(config, *arch, *revision); err != nil {
		return err
	}
	layout := filepath.Join(tmp, "layout")
	if _, err := command("skopeo", "copy", source, "oci:"+layout+":candidate"); err != nil {
		return err
	}
	bundle := filepath.Join(tmp, "bundle")
	if _, err := command("umoci", "unpack", "--rootless", "--image", layout+":candidate", bundle); err != nil {
		return err
	}
	root := filepath.Join(bundle, "rootfs")
	// This is userspace qualification, not a boot test. No engine socket,
	// privileges, mounts or host device access are needed.
	checks := []struct {
		name string
		args []string
	}{
		{"Fedora version", []string{"/usr/bin/bash", "-ec", `. /usr/lib/os-release; test "$VERSION_ID" = "$1"`, "check", *version}},
		{"Niri configuration", []string{"/usr/bin/niri", "validate", "--config", "/etc/niri/config.kdl"}},
		{"desktop and keyring packages", []string{"/usr/bin/rpm", "-q", "greetd", "pam_oo7", "niri", "dms", "dms-greeter", "quickshell", "mesa-vulkan-drivers"}},
		{"PAM dynamic dependencies", []string{"/usr/bin/bash", "-ec", `output=$(ldd /usr/lib64/security/pam_oo7.so); printf '%s\n' "$output"; ! grep -q 'not found' <<< "$output"`}},
		{"enabled greeter", []string{"/usr/bin/systemctl", "--root=/", "is-enabled", "greetd.service"}},
		{"enabled GPU profile", []string{"/usr/bin/systemctl", "--root=/", "is-enabled", "niri-strix-halo-128g-memory.service"}},
		{"GPU helper syntax", []string{"/usr/bin/bash", "-n", "/usr/libexec/niri-strix-halo-memory"}},
		{"GPU memory ceiling", []string{"/usr/bin/grep", "-Fx", "Environment=TTM_PAGES_LIMIT=20971520", "/usr/lib/systemd/system/niri-strix-halo-128g-memory.service"}},
	}
	results := map[string]string{}
	for _, check := range checks {
		data, err := command("chroot", append([]string{root}, check.args...)...)
		results[check.name] = string(data)
		if err != nil {
			return fmt.Errorf("%s: %w: %s", check.name, err, data)
		}
	}
	inventory, err := command("chroot", root, "/usr/bin/rpm", "-qa", "--qf", "%{NAME}|%{EVR}|%{ARCH}\n")
	if err != nil {
		return err
	}
	after, err := hash(archivePath)
	if err != nil {
		return err
	}
	if after != before {
		return fmt.Errorf("archive changed during qualification")
	}
	report := map[string]any{
		"schema_version": 1, "source_sha": *revision, "arch": *arch,
		"archive": filepath.Base(*archive), "sha256": before, "tested": true,
		"qualification": "userspace", "boot_tested": false,
		"run_id": os.Getenv("GITHUB_RUN_ID"), "run_attempt": os.Getenv("GITHUB_RUN_ATTEMPT"),
		"checks": results, "packages": strings.Split(strings.TrimSpace(string(inventory)), "\n"),
	}
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(outputPath), 0755); err != nil {
		return err
	}
	if err := os.WriteFile(outputPath, append(data, '\n'), 0644); err != nil {
		return err
	}
	return workflow.Output("receipt", *output)
}

func main() { workflow.Main("test-atomic-image 1", run) }
