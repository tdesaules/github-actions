package binaryrelease

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

type Release struct {
	ID         int64  `json:"id"`
	Tag        string `json:"tag_name"`
	Draft      bool   `json:"draft"`
	Prerelease bool   `json:"prerelease"`
}
type Asset struct {
	ID     int64  `json:"id"`
	Name   string `json:"name"`
	Size   int64  `json:"size"`
	Digest string `json:"digest"`
}

func (a API) latest(version string) (bool, error) {
	for page := 1; page <= 1000; page++ {
		var batch []Release
		_, e := a.json("GET", fmt.Sprintf("/releases?per_page=100&page=%d", page), nil, &batch)
		if e != nil {
			return false, e
		}
		for _, r := range batch {
			if !r.Draft && !r.Prerelease && stable.MatchString(r.Tag) && VersionCompare(r.Tag, version) > 0 {
				return false, nil
			}
		}
		if len(batch) < 100 {
			return true, nil
		}
	}
	return false, fmt.Errorf("release pagination limit exceeded")
}
func (a API) assets(release int64) ([]Asset, error) {
	all := []Asset{}
	for page := 1; page <= 1000; page++ {
		var batch []Asset
		_, e := a.json("GET", fmt.Sprintf("/releases/%d/assets?per_page=100&page=%d", release, page), nil, &batch)
		if e != nil {
			return nil, e
		}
		all = append(all, batch...)
		if len(batch) < 100 {
			return all, nil
		}
	}
	return nil, fmt.Errorf("asset pagination limit exceeded")
}
func (a API) upload(release int64, path string) error {
	f, e := os.Open(path)
	if e != nil {
		return e
	}
	defer f.Close()
	st, e := f.Stat()
	if e != nil {
		return e
	}
	if !st.Mode().IsRegular() || st.Size() > maxDownload {
		return fmt.Errorf("asset too large or not regular")
	}
	base := a.Base
	if base == "https://api.github.com" {
		base = "https://uploads.github.com"
	}
	req, e := http.NewRequest("POST", fmt.Sprintf("%s/repos/%s/releases/%d/assets?name=%s", base, a.Repository, release, url.QueryEscape(filepath.Base(path))), f)
	if e != nil {
		return fmt.Errorf("invalid upload request")
	}
	req.ContentLength = st.Size()
	req.Header.Set("Authorization", "Bearer "+a.Token)
	req.Header.Set("Content-Type", "application/octet-stream")
	r, e := a.Client.Do(req)
	if e != nil {
		return fmt.Errorf("asset upload failed")
	}
	defer r.Body.Close()
	if r.StatusCode != 201 {
		return fmt.Errorf("asset upload HTTP %d", r.StatusCode)
	}
	return nil
}
func (a API) verifyAsset(asset Asset, path string) error {
	sum, e := Hash(path)
	if e != nil {
		return e
	}
	st, e := os.Stat(path)
	if e != nil {
		return e
	}
	if asset.Size != st.Size() {
		return fmt.Errorf("existing release asset differs")
	}
	if asset.Digest != "" {
		if asset.Digest != "sha256:"+sum {
			return fmt.Errorf("existing release asset differs")
		}
		return nil
	}
	// Older GitHub assets may lack digest metadata: compare downloaded bytes.
	req, e := http.NewRequest("GET", fmt.Sprintf("%s/repos/%s/releases/assets/%d", a.Base, a.Repository, asset.ID), nil)
	if e != nil {
		return fmt.Errorf("invalid asset request")
	}
	req.Header.Set("Authorization", "Bearer "+a.Token)
	req.Header.Set("Accept", "application/octet-stream")
	r, e := a.Client.Do(req)
	if e != nil {
		return fmt.Errorf("asset download failed")
	}
	if r.StatusCode == 302 {
		location := r.Header.Get("Location")
		r.Body.Close()
		u, e := url.Parse(location)
		if e != nil || u.Scheme != "https" || u.User != nil {
			return fmt.Errorf("invalid asset redirect")
		}
		request, _ := http.NewRequest("GET", location, nil)
		r, e = a.Client.Do(request)
		if e != nil {
			return fmt.Errorf("asset download failed")
		}
	}
	defer r.Body.Close()
	if r.StatusCode != 200 {
		return fmt.Errorf("asset download HTTP %d", r.StatusCode)
	}
	tmp, e := os.CreateTemp("", "release-asset-")
	if e != nil {
		return e
	}
	defer os.Remove(tmp.Name())
	n, e := io.Copy(tmp, io.LimitReader(r.Body, maxDownload+1))
	ce := tmp.Close()
	if e != nil {
		return e
	}
	if ce != nil {
		return ce
	}
	if n != st.Size() {
		return fmt.Errorf("existing release asset differs")
	}
	download, e := Hash(tmp.Name())
	if e != nil {
		return e
	}
	if sum != download {
		return fmt.Errorf("existing release asset differs")
	}
	return nil
}
func (a API) Publish(tag, notes string, files []string) error {
	if !stable.MatchString(tag) {
		return fmt.Errorf("invalid release tag")
	}
	newest, e := a.latest(tag)
	if e != nil {
		return e
	}
	latest := "false"
	if newest {
		latest = "true"
	}
	body := map[string]any{"tag_name": tag, "name": tag, "body": notes, "draft": false, "prerelease": false, "make_latest": latest}
	var release Release
	status, e := a.json("GET", "/releases/tags/"+tag, nil, &release)
	if e != nil && status != 404 {
		return e
	}
	if status == 404 {
		_, e = a.json("POST", "/releases", body, &release)
	} else {
		if release.Draft || release.Prerelease || release.Tag != tag {
			return fmt.Errorf("existing release policy mismatch")
		}
		_, e = a.json("PATCH", fmt.Sprintf("/releases/%d", release.ID), body, &release)
	}
	if e != nil {
		return e
	}
	assets, e := a.assets(release.ID)
	if e != nil {
		return e
	}
	for _, path := range files {
		matches := 0
		for _, asset := range assets {
			if asset.Name == filepath.Base(path) {
				matches++
				if e = a.verifyAsset(asset, path); e != nil {
					return e
				}
			}
		}
		if matches > 1 {
			return fmt.Errorf("duplicate existing release assets")
		}
		if matches == 0 {
			if e = a.upload(release.ID, path); e != nil {
				return e
			}
		}
	}
	return nil
}
func PublishAction() error {
	tag := Input("tag", strings.TrimPrefix(os.Getenv("GITHUB_REF"), "refs/tags/"))
	m, final, e := ReadTag(tag)
	if e != nil {
		return e
	}
	if e = Head(final); e != nil {
		return e
	}
	a, e := NewAPI()
	if e != nil {
		return e
	}
	r, e := a.RunAttempt(m.RunID, m.RunAttempt)
	if e != nil {
		return e
	}
	if e = checkRun(r, m, true); e != nil {
		return e
	}
	q, e := Verify(Input("qualified-directory", ".build/qualified"), m.SourceSHA, m.Version, m.RunID, m.RunAttempt)
	if e != nil {
		return e
	}
	if e = MatchBindings(q, m); e != nil {
		return e
	}
	tmp, e := os.MkdirTemp("", "binary-publish-")
	if e != nil {
		return e
	}
	defer os.RemoveAll(tmp)
	if e = a.Fetch(m, filepath.Join(tmp, "original"), true); e != nil {
		return e
	}
	files := []string{}
	sums := ""
	for _, v := range q {
		files = append(files, v.Path)
		sums += v.SHA256 + "  " + v.Archive + "\n"
	}
	sumPath := filepath.Join(tmp, "SHA256SUMS")
	if e = os.WriteFile(sumPath, []byte(sums), 0644); e != nil {
		return e
	}
	files = append(files, sumPath)
	evidence := filepath.Join(tmp, "release-evidence.json")
	if e = WriteJSON(evidence, struct {
		Marker
		FinalSHA string `json:"final_sha"`
	}{m, final}); e != nil {
		return e
	}
	files = append(files, evidence)
	notes, e := os.ReadFile("CHANGELOG.md")
	if e != nil {
		return e
	}
	if e = a.Publish(tag, string(notes), files); e != nil {
		return e
	}
	return Output("tag", tag)
}
