package main

import (
	"crypto/sha256"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/tdesaules/github-actions/shared/workflow"
)

func platforms(raw []byte) (map[string]string, error) {
	var index struct {
		SchemaVersion int    `json:"schemaVersion"`
		MediaType     string `json:"mediaType"`
		Manifests     []struct {
			Digest   string `json:"digest"`
			Platform struct {
				Architecture string `json:"architecture"`
				OS           string `json:"os"`
			} `json:"platform"`
		} `json:"manifests"`
	}
	if err := json.Unmarshal(raw, &index); err != nil {
		return nil, err
	}
	if index.SchemaVersion != 2 || (index.MediaType != "application/vnd.oci.image.index.v1+json" &&
		index.MediaType != "application/vnd.docker.distribution.manifest.list.v2+json") {
		return nil, fmt.Errorf("OCI index or Docker manifest list required")
	}
	result := map[string]string{}
	for _, item := range index.Manifests {
		if item.Platform.OS != "linux" {
			continue // BuildKit attestation descriptors are not runnable platforms.
		}
		arch := item.Platform.Architecture
		if arch != "amd64" && arch != "arm64" {
			return nil, fmt.Errorf("unsupported native platform %q", arch)
		}
		if result[arch] != "" || !regexp.MustCompile(`^sha256:[a-f0-9]{64}$`).MatchString(item.Digest) {
			return nil, fmt.Errorf("duplicate platform or invalid native manifest digest")
		}
		result[arch] = item.Digest
	}
	if len(result) != 2 {
		return nil, fmt.Errorf("index must contain both AMD64 and ARM64")
	}
	return result, nil
}

func run() error {
	image := flag.String("image", "", "GHCR image tag or immutable index")
	username := flag.String("registry-username", "", "optional registry user")
	flag.Parse()
	if flag.NArg() != 0 ||
		!regexp.MustCompile(`^ghcr\.io/[a-z0-9][a-z0-9._/-]*(?::[a-zA-Z0-9_][a-zA-Z0-9_.-]*|@sha256:[a-f0-9]{64})$`).MatchString(*image) {
		return fmt.Errorf("explicit GHCR tag or immutable index required")
	}
	private, err := os.MkdirTemp("", "image-platforms-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(private)
	auth := filepath.Join(private, "auth.json")
	args := []string{"inspect", "--raw"}
	password := os.Getenv("INPUT_REGISTRY_PASSWORD")
	if password != "" {
		if *username == "" {
			return fmt.Errorf("registry username required")
		}
		login := exec.Command("skopeo", "login", "--username", *username,
			"--password-stdin", "--authfile", auth, "ghcr.io")
		login.Stdin = strings.NewReader(password)
		login.Stdout, login.Stderr = os.Stdout, os.Stderr
		if err := login.Run(); err != nil {
			return err
		}
		args = append(args, "--authfile", auth)
	}
	args = append(args, "docker://"+*image)
	cmd := exec.Command("skopeo", args...)
	cmd.Stderr = os.Stderr
	raw, err := cmd.Output()
	if err != nil {
		return err
	}
	native, err := platforms(raw)
	if err != nil {
		return err
	}
	digest := fmt.Sprintf("sha256:%x", sha256.Sum256(raw))
	repository := *image
	if at := strings.IndexByte(repository, '@'); at >= 0 {
		if repository[at+1:] != digest {
			return fmt.Errorf("returned index digest does not match requested identity")
		}
		repository = repository[:at]
	} else {
		repository = repository[:strings.LastIndexByte(repository, ':')]
	}
	for name, value := range map[string]string{
		"reference": repository + "@" + digest, "digest": digest,
		"image-amd64":  repository + "@" + native["amd64"],
		"image-arm64":  repository + "@" + native["arm64"],
		"digest-amd64": native["amd64"], "digest-arm64": native["arm64"],
	} {
		if err := workflow.Output(name, value); err != nil {
			return err
		}
	}
	return nil
}

func main() { workflow.Main("get-image-platforms 1", run) }
