package publication

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/tdesaules/github-actions/shared/catalog"
	action "github.com/tdesaules/github-actions/shared/runtime"
	"github.com/tdesaules/github-actions/shared/workflow"
)

func References(image *Image) []string {
	repo := strings.Split(image.Reference, "@")[0]
	return []string{repo + "@" + image.Platforms["amd64"], repo + "@" + image.Platforms["arm64"], image.Reference}
}

var requestOIDCToken = action.OIDCToken

func Sign(directory string) error {
	cleanup, err := Auth()
	if err != nil {
		return err
	}
	defer cleanup()
	state, err := Load(directory)
	if err != nil {
		return err
	}
	owners, err := catalog.Discover()
	if err != nil {
		return err
	}
	for _, owner := range owners {
		image := state.Images[owner.Name]
		if !image.Published {
			return fmt.Errorf("cannot sign before verified publication")
		}
		if image.Reused {
			continue
		}
		for _, reference := range References(image) {
			token, err := requestOIDCToken()
			if err != nil {
				return err
			}
			if err = catalog.Run("cosign", "sign", "--yes", "--identity-token", token, reference); err != nil {
				return err
			}
		}
	}
	return nil
}
func Verify(directory, tag, repository string) error {
	if !regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$`).MatchString(repository) {
		return fmt.Errorf("invalid repository")
	}
	cleanup, err := Auth()
	if err != nil {
		return err
	}
	defer cleanup()
	state := State{Images: map[string]*Image{}}
	owners, err := catalog.Discover()
	if err != nil {
		return err
	}
	if tag != "" {
		if !SemVer.MatchString(tag) {
			return fmt.Errorf("release tag must be SemVer without v")
		}
		head, err := Revision()
		if err != nil {
			return err
		}
		target, err := workflow.Git("rev-parse", "--verify", "--end-of-options", "refs/tags/"+tag+"^{commit}")
		if err != nil {
			return err
		}
		if strings.TrimSpace(target) != head {
			return fmt.Errorf("release tag differs from checkout")
		}
		if _, err = workflow.Git("merge-base", "--is-ancestor", "HEAD", "origin/main"); err != nil {
			return err
		}
		for _, owner := range owners {
			if !catalog.Pinned.MatchString(owner.Reference) {
				return fmt.Errorf("unpublished release image")
			}
			raw, err := Raw(owner.Reference)
			if err != nil {
				return err
			}
			if Hash(raw) != strings.Split(owner.Reference, "@")[1] {
				return fmt.Errorf("release image digest mismatch")
			}
			children, err := Children(raw)
			if err != nil {
				return err
			}
			state.Images[owner.Name] = &Image{Reference: owner.Reference, Platforms: children, Published: true}
		}
	} else {
		state, err = Load(directory)
		if err != nil {
			return err
		}
		for _, image := range state.Images {
			image.Signed = false
		}
		if err = Save(directory, state); err != nil {
			return err
		}
	}
	identity := "https://github.com/" + repository + "/.github/workflows/main.yml@refs/heads/main"
	for _, owner := range owners {
		image := state.Images[owner.Name]
		if !image.Published {
			return fmt.Errorf("cannot verify unpublished image")
		}
		for _, reference := range References(image) {
			cmd := exec.Command("cosign", "verify", "--output=json", "--certificate-identity", identity, "--certificate-oidc-issuer", "https://token.actions.githubusercontent.com", reference)
			cmd.Stderr = os.Stderr
			data, err := cmd.Output()
			if err != nil {
				return err
			}
			path := filepath.Join(directory, "signatures", owner.Name+"-"+strings.Split(reference, "sha256:")[1]+".json")
			if err = os.MkdirAll(filepath.Dir(path), 0755); err != nil {
				return err
			}
			if err = os.WriteFile(path, data, 0644); err != nil {
				return err
			}
		}
		image.Signed = true
		if tag == "" {
			if err = Save(directory, state); err != nil {
				return err
			}
		}
	}
	return nil
}
