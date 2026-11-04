package pantavisor

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/tdesaules/github-actions/shared/component"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"
)

func NewGitHubRelease() error {
	receipt, expected, componentRef, imageRef := Input("receipt", "artifacts/qualified-receipt.json"), Input("receipt-digest", ""), Input("component", ""), Input("image", "")
	if err := Parse(); err != nil {
		return err
	}
	sum, err := component.Hash(*receipt)
	if err != nil || sum != *expected {
		return fmt.Errorf("qualification receipt differs from the release tag")
	}
	revision := os.Getenv("GITHUB_SHA")
	value, err := CheckReceipt("dist", *receipt, revision, true)
	if err != nil {
		return err
	}
	if os.Getenv("GITHUB_REF") != "refs/tags/v"+value.Version {
		return fmt.Errorf("tag/version mismatch")
	}
	reference := regexp.MustCompile(`^ghcr\.io/[a-z0-9._/-]+@sha256:[a-f0-9]{64}$`)
	if !reference.MatchString(*componentRef) || (*imageRef != "" && !reference.MatchString(*imageRef)) {
		return fmt.Errorf("immutable publication references required")
	}
	repository, token := os.Getenv("GITHUB_REPOSITORY"), os.Getenv("INPUT_TOKEN")
	if !regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$`).MatchString(repository) || token == "" {
		return fmt.Errorf("repository and GitHub token required")
	}
	manifest := map[string]any{"schema": 1, "project": value.Project, "version": value.Version, "revision": value.Revision, "component": *componentRef, "image": *imageRef, "qualificationReceiptSha256": sum}
	if value.Project == "system" {
		var inputs map[string]any
		if err := ReadJSON("compositions/components.json", &inputs); err != nil {
			return err
		}
		for _, name := range []string{"core", "k0s"} {
			ref, ok := inputs[name].(string)
			if !ok || !reference.MatchString(ref) {
				return fmt.Errorf("System release requires immutable component inputs")
			}
		}
		bundles := map[string]string{}
		for _, profile := range []string{"base", "k0s"} {
			digest, err := component.Hash("dist/platform-" + profile + "/bundle.json")
			if err != nil {
				return err
			}
			bundles[profile] = "sha256:" + digest
		}
		manifest["inputs"], manifest["platformDescriptors"], manifest["profiles"] = inputs, bundles, value.Profiles
	}
	data, _ := json.MarshalIndent(manifest, "", "  ")
	body := "Qualified Pantavisor release.\n\n```json\n" + string(data) + "\n```\n"
	if err := WriteJSON("artifacts/release-manifest.json", manifest); err != nil {
		return err
	}
	client := http.Client{Timeout: 60 * time.Second}
	base := "https://api.github.com/repos/" + repository + "/releases"
	call := func(method, url string, payload any) (int, map[string]any, error) {
		var input io.Reader
		if payload != nil {
			b, e := json.Marshal(payload)
			if e != nil {
				return 0, nil, e
			}
			input = bytes.NewReader(b)
		}
		req, e := http.NewRequest(method, url, input)
		if e != nil {
			return 0, nil, e
		}
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Accept", "application/vnd.github+json")
		req.Header.Set("Content-Type", "application/json")
		res, e := client.Do(req)
		if e != nil {
			return 0, nil, e
		}
		defer res.Body.Close()
		var result map[string]any
		e = json.NewDecoder(res.Body).Decode(&result)
		return res.StatusCode, result, e
	}
	tag := "v" + value.Version
	status, existing, err := call("GET", base+"/tags/"+tag, nil)
	if err != nil {
		return err
	}
	if status == 200 {
		if existing["body"] != body || existing["draft"] != false {
			return fmt.Errorf("existing GitHub Release differs")
		}
		return nil
	}
	if status != 404 {
		return fmt.Errorf("GitHub release lookup failed: HTTP %d", status)
	}
	if os.Getenv("GITHUB_REF") != "refs/tags/"+tag {
		return fmt.Errorf("tag/version mismatch")
	}
	status, _, err = call("POST", base, map[string]any{"tag_name": tag, "name": tag, "body": body, "draft": false, "prerelease": false, "target_commitish": revision})
	if err != nil {
		return err
	}
	if status != 201 {
		return fmt.Errorf("GitHub release creation failed: HTTP %d", status)
	}
	return nil
}

func ReadComponents() error {
	path := Input("file", "compositions/components.json")
	if err := Parse(); err != nil {
		return err
	}
	var references struct {
		Schema int    `json:"schema"`
		Core   string `json:"core"`
		K0s    string `json:"k0s"`
	}
	if err := ReadJSON(*path, &references); err != nil {
		return err
	}
	pattern := regexp.MustCompile(`^ghcr\.io/[a-z0-9._/-]+@sha256:[a-f0-9]{64}$`)
	if references.Schema != 1 || !pattern.MatchString(references.Core) || !pattern.MatchString(references.K0s) {
		return fmt.Errorf("versioned Core and k0s references must be pinned GHCR component digests")
	}
	// Keep this operation independent from downloading or publishing components.
	for key, value := range map[string]string{"core": references.Core, "k0s": references.K0s} {
		if err := outputReference(key, strings.TrimSpace(value)); err != nil {
			return err
		}
	}
	return nil
}
