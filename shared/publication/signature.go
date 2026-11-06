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

type signatureResult struct {
	Reference string `json:"subject_digest"`
	Operation string `json:"operation"`
	Status    string `json:"status"`
	Reason    string `json:"reason,omitempty"`
}

// ValidateTagCatalog is functional provenance validation, independent of Cosign.
func ValidateTagCatalog(tag string) (State, error) {
	state := State{Images: map[string]*Image{}}
	if !SemVer.MatchString(tag) {
		return state, fmt.Errorf("release tag must be SemVer without v")
	}
	head, err := Revision()
	if err != nil {
		return state, err
	}
	target, err := workflow.Git("rev-parse", "--verify", "--end-of-options", "refs/tags/"+tag+"^{commit}")
	if err != nil {
		return state, err
	}
	if strings.TrimSpace(target) != head {
		return state, fmt.Errorf("release tag differs from checkout")
	}
	if _, err = workflow.Git("merge-base", "--is-ancestor", "HEAD", "origin/main"); err != nil {
		return state, err
	}
	owners, err := catalog.Discover()
	if err != nil {
		return state, err
	}
	for _, owner := range owners {
		if !catalog.Pinned.MatchString(owner.Reference) {
			return state, fmt.Errorf("unpublished release image")
		}
		raw, err := Raw(owner.Reference)
		if err != nil {
			return state, err
		}
		if Hash(raw) != strings.Split(owner.Reference, "@")[1] {
			return state, fmt.Errorf("release image digest mismatch")
		}
		children, err := Children(raw)
		if err != nil {
			return state, err
		}
		state.Images[owner.Name] = &Image{Reference: owner.Reference, Platforms: children, Published: true}
	}
	return state, nil
}

func Sign(directory string) (result error) {
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
	reports := map[string]*signatureResult{}
	for _, image := range state.Images {
		for _, reference := range References(image) {
			reports[reference] = &signatureResult{Reference: reference, Operation: "cosign sign", Status: "not_run"}
			if image.Reused {
				reports[reference].Reason = "Reused digest; signing not requested"
			}
		}
	}
	defer func() {
		if err := catalog.Write(filepath.Join(directory, "signing-status.json"), reports); result == nil {
			result = err
		}
	}()
	for _, owner := range owners {
		image := state.Images[owner.Name]
		if !image.Published {
			return fmt.Errorf("cannot sign before verified publication")
		}
		if image.Reused {
			continue
		}
		for _, reference := range References(image) {
			report := reports[reference]
			report.Status = "error"
			report.Reason = "Signing did not complete; see step diagnostics"
			token, err := requestOIDCToken()
			if err != nil {
				return err
			}
			path, cleanup, err := action.IdentityTokenFile(token)
			if err != nil {
				return err
			}
			err = catalog.Run("cosign", "sign", "--yes", "--identity-token", path, reference)
			cleanup()
			if err != nil {
				return err
			}
			report.Status, report.Reason = "signed", ""
		}
	}
	return nil
}
func Verify(directory, tag, repository string) (result error) {
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
		state, err = ValidateTagCatalog(tag)
		if err != nil {
			return err
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
	reports := map[string]*signatureResult{}
	for _, image := range state.Images {
		for _, reference := range References(image) {
			reports[reference] = &signatureResult{Reference: reference, Operation: "cosign verify", Status: "not_run"}
		}
	}
	defer func() {
		if err := catalog.Write(filepath.Join(directory, "verification-status.json"), reports); result == nil {
			result = err
		}
	}()
	for _, owner := range owners {
		image := state.Images[owner.Name]
		if !image.Published {
			return fmt.Errorf("cannot verify unpublished image")
		}
		for _, reference := range References(image) {
			report := reports[reference]
			report.Status, report.Reason = "error", "Verification did not complete; see step diagnostics"
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
			report.Status, report.Reason = "verified", ""
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
