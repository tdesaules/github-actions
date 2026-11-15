package atomicrelease

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type Remote struct {
	Client                                     *http.Client
	APIURL, RegistryURL                        string
	Token, RegistryToken, Username, Repository string
}

func NewRemote(repository, token, registryToken, username string) (*Remote, error) {
	if !repositoryName.MatchString(repository) || token == "" {
		return nil, fmt.Errorf("repository and job-local token required")
	}
	return &Remote{
		Client: &http.Client{Timeout: 120 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return fmt.Errorf("redirect refused") }},
		APIURL: "https://api.github.com", RegistryURL: "https://ghcr.io", Repository: repository,
		Token: token, RegistryToken: registryToken, Username: username,
	}, nil
}
func (a *Remote) request(method, endpoint, token, media string, body []byte) ([]byte, int, error) {
	req, err := http.NewRequest(method, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, 0, err
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	req.Header.Set("Accept", "application/vnd.github+json, application/vnd.oci.image.index.v1+json, application/vnd.oci.image.manifest.v1+json, application/vnd.docker.distribution.manifest.list.v2+json, application/vnd.docker.distribution.manifest.v2+json")
	if media != "" {
		req.Header.Set("Content-Type", media)
	}
	res, err := a.Client.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("remote request failed")
	}
	defer res.Body.Close()
	data, err := io.ReadAll(io.LimitReader(res.Body, 8<<20))
	if err != nil {
		return nil, res.StatusCode, err
	}
	return data, res.StatusCode, nil
}
func (a *Remote) api(method, path string, v any) ([]byte, int, error) {
	var body []byte
	if v != nil {
		var err error
		body, err = json.Marshal(v)
		if err != nil {
			return nil, 0, err
		}
	}
	return a.request(method, a.APIURL+path, a.Token, "application/json", body)
}

// VerifyAppToken verifies installation-token repository access. The workflow must
// obtain it using the official App-token action with the configured bot App.
func (a *Remote) VerifyAppToken() error {
	for page := 1; ; page++ {
		raw, status, err := a.api("GET", fmt.Sprintf("/installation/repositories?per_page=100&page=%d", page), nil)
		if err != nil {
			return err
		}
		if status != 200 {
			return fmt.Errorf("repository-scoped App installation token required")
		}
		var response struct {
			Repositories []struct {
				FullName string `json:"full_name"`
			} `json:"repositories"`
		}
		if err = json.Unmarshal(raw, &response); err != nil {
			return err
		}
		for _, r := range response.Repositories {
			if strings.EqualFold(r.FullName, a.Repository) {
				return nil
			}
		}
		if len(response.Repositories) < 100 {
			return fmt.Errorf("App installation token does not authorize this repository")
		}
	}
}
func (a *Remote) registryAuth(repository, actions string) (string, error) {
	if a.RegistryToken == "" || a.Username == "" {
		return "", fmt.Errorf("job-local GHCR credentials required")
	}
	req, err := http.NewRequest("GET", a.RegistryURL+"/token?service=ghcr.io&scope="+url.QueryEscape("repository:"+repository+":"+actions), nil)
	if err != nil {
		return "", err
	}
	req.SetBasicAuth(a.Username, a.RegistryToken)
	res, err := a.Client.Do(req)
	if err != nil {
		return "", fmt.Errorf("registry authentication failed")
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return "", fmt.Errorf("registry authentication status %d", res.StatusCode)
	}
	var result struct {
		Token string `json:"token"`
	}
	if err = json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(&result); err != nil {
		return "", err
	}
	if result.Token == "" {
		return "", fmt.Errorf("empty registry token")
	}
	return result.Token, nil
}

type descriptor struct {
	MediaType   string                            `json:"mediaType"`
	Digest      string                            `json:"digest"`
	Size        int64                             `json:"size"`
	Platform    struct{ Architecture, OS string } `json:"platform"`
	Annotations map[string]string                 `json:"annotations"`
}
type manifest struct {
	ArtifactType string       `json:"artifactType,omitempty"`
	MediaType    string       `json:"mediaType"`
	Manifests    []descriptor `json:"manifests"`
	Layers       []descriptor `json:"layers"`
}

func (a *Remote) getManifest(repo, ref, token string) ([]byte, error) {
	b, status, err := a.request("GET", a.RegistryURL+"/v2/"+repo+"/manifests/"+ref, token, "", nil)
	if err != nil {
		return nil, err
	}
	if status != 200 {
		return nil, fmt.Errorf("manifest retrieval status %d", status)
	}
	if digest.MatchString(ref) && fmt.Sprintf("sha256:%x", sha256.Sum256(b)) != ref {
		return nil, fmt.Errorf("registry manifest digest mismatch")
	}
	return b, nil
}
func verifyIndex(raw []byte, b Bundle) error {
	var index manifest
	if err := json.Unmarshal(raw, &index); err != nil {
		return err
	}
	if index.MediaType != "application/vnd.oci.image.index.v1+json" && index.MediaType != "application/vnd.docker.distribution.manifest.list.v2+json" {
		return fmt.Errorf("unsupported OS index media type")
	}
	if len(index.Manifests) != 2 {
		return fmt.Errorf("published OS index must have exactly two native manifests")
	}
	wanted := map[string]string{"amd64": b.AMD64.ManifestDigest, "arm64": b.ARM64.ManifestDigest}
	for _, d := range index.Manifests {
		if d.Platform.OS != "linux" || wanted[d.Platform.Architecture] == "" || wanted[d.Platform.Architecture] != d.Digest {
			return fmt.Errorf("published index differs from qualified platforms")
		}
		delete(wanted, d.Platform.Architecture)
	}
	return nil
}
func verifyISO(raw []byte, r Receipt, files []Payload) error {
	var m manifest
	if err := json.Unmarshal(raw, &m); err != nil {
		return err
	}
	if m.ArtifactType != "application/vnd.niri-tonic.installer.v1" {
		return fmt.Errorf("unexpected installer artifact type")
	}
	if len(m.Layers) != len(files) {
		return fmt.Errorf("published ISO payload set differs")
	}
	wanted := map[string]Payload{}
	for _, f := range files {
		wanted[f.Path] = f
	}
	found := false
	for _, d := range m.Layers {
		name := d.Annotations["org.opencontainers.image.title"]
		f, ok := wanted[name]
		if !ok || d.Digest != "sha256:"+f.SHA256 || d.MediaType != f.MediaType {
			return fmt.Errorf("published ISO payload checksum or media type differs")
		}
		delete(wanted, name)
		if d.Annotations["org.opencontainers.image.title"] == r.Archive {
			if found || d.Digest != "sha256:"+r.SHA256 || d.Size != r.Size || d.MediaType != "application/x-iso9660-image" {
				return fmt.Errorf("published ISO differs from qualified payload")
			}
			found = true
		}
	}
	if !found {
		return fmt.Errorf("published ISO payload absent")
	}
	return nil
}

// VerifyPublished binds publication outputs to the qualified platform manifests
// and the checksum/size of the ORAS ISO payload.
func (a *Remote) VerifyPublished(e Evidence) ([]byte, error) {
	if err := ValidateEvidence(e, e.Version, a.Repository); err != nil {
		return nil, err
	}
	token, err := a.registryAuth(a.Repository, "pull")
	if err != nil {
		return nil, err
	}
	raw, err := a.getManifest(a.Repository, strings.Split(e.Image, "@")[1], token)
	if err != nil {
		return nil, err
	}
	if err = verifyIndex(raw, e.Qualified); err != nil {
		return nil, err
	}
	token, err = a.registryAuth(a.Repository+"-iso", "pull")
	if err != nil {
		return nil, err
	}
	iso, err := a.getManifest(a.Repository+"-iso", strings.Split(e.ISO, "@")[1], token)
	if err != nil {
		return nil, err
	}
	if err = verifyISO(iso, e.Qualified.ISO, e.ISOFiles); err != nil {
		return nil, err
	}
	return raw, nil
}
func (a *Remote) Promote(raw []byte) error {
	var m manifest
	if err := json.Unmarshal(raw, &m); err != nil {
		return err
	}
	if m.MediaType != "application/vnd.oci.image.index.v1+json" && m.MediaType != "application/vnd.docker.distribution.manifest.list.v2+json" {
		return fmt.Errorf("OS index media type required")
	}
	token, err := a.registryAuth(a.Repository, "pull,push")
	if err != nil {
		return err
	}
	// The consumer's Fedora generation is intentionally fixed, not arbitrary tags.
	for _, channel := range []string{"45", "latest"} {
		_, status, err := a.request("PUT", a.RegistryURL+"/v2/"+a.Repository+"/manifests/"+channel, token, m.MediaType, raw)
		if err != nil {
			return err
		}
		if status != 201 {
			return fmt.Errorf("channel promotion status %d", status)
		}
		got, err := a.getManifest(a.Repository, channel, token)
		if err != nil {
			return err
		}
		if !bytes.Equal(got, raw) {
			return fmt.Errorf("channel promotion round-trip mismatch")
		}
	}
	return nil
}
func releaseBody(e Evidence, final, notes string) string {
	return notes + "\n\n## Qualified artifacts\n\n- OS: `" + e.Image + "`\n- Installer: `" + e.ISO + "`\n- ISO SHA-256: `" + e.ISOSHA256 + "`\n- Qualified source: `" + e.SourceSHA + "`\n- Release metadata: `" + final + "`\n\nUserspace and installer-media qualified; boot testing is separate local evidence.\n"
}
func (a *Remote) CheckRelease(e Evidence, final, notes string) (string, error) {
	path := "/repos/" + a.Repository + "/releases"
	body := releaseBody(e, final, notes)
	raw, status, err := a.api("GET", path+"/tags/"+e.Version, nil)
	if err != nil {
		return "", err
	}
	if status == 200 {
		var existing struct {
			Tag, Target, Body, URL string
			Draft, Prerelease      bool
		}
		var fields map[string]json.RawMessage
		if err = json.Unmarshal(raw, &fields); err != nil {
			return "", err
		}
		_ = json.Unmarshal(fields["tag_name"], &existing.Tag)
		_ = json.Unmarshal(fields["target_commitish"], &existing.Target)
		_ = json.Unmarshal(fields["body"], &existing.Body)
		_ = json.Unmarshal(fields["html_url"], &existing.URL)
		_ = json.Unmarshal(fields["draft"], &existing.Draft)
		_ = json.Unmarshal(fields["prerelease"], &existing.Prerelease)
		if existing.Tag != e.Version || existing.Target != final || existing.Body != body || existing.Draft || existing.Prerelease || existing.URL == "" {
			return "", fmt.Errorf("existing GitHub Release conflicts with qualified identity")
		}
		return existing.URL, nil
	}
	if status != 404 {
		return "", fmt.Errorf("release lookup status %d", status)
	}
	return "", nil
}
func (a *Remote) Release(e Evidence, final, notes string, latest bool) (string, error) {
	existing, err := a.CheckRelease(e, final, notes)
	if err != nil || existing != "" {
		return existing, err
	}
	path := "/repos/" + a.Repository + "/releases"
	raw, status, err := a.api("POST", path, map[string]any{"tag_name": e.Version, "target_commitish": final, "name": e.Version, "body": releaseBody(e, final, notes), "draft": false, "prerelease": false, "make_latest": fmt.Sprint(latest)})
	if err != nil {
		return "", err
	}
	if status != 201 {
		return "", fmt.Errorf("GitHub Release publication status %d", status)
	}
	var response struct {
		URL string `json:"html_url"`
	}
	if err = json.Unmarshal(raw, &response); err != nil {
		return "", err
	}
	if strings.TrimSpace(response.URL) == "" {
		return "", fmt.Errorf("GitHub Release publication response lacks html_url")
	}
	return response.URL, nil
}
