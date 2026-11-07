package binaryrelease

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const maxDownload int64 = 2 << 30

type API struct {
	Base, Repository, Token string
	Client                  *http.Client
}

func NewAPI() (API, error) {
	a := API{Base: "https://api.github.com", Repository: Input("repository", os.Getenv("GITHUB_REPOSITORY")), Token: Input("token", os.Getenv("GH_TOKEN")), Client: &http.Client{Timeout: 10 * time.Minute, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}}
	if !regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$`).MatchString(a.Repository) || a.Token == "" {
		return a, fmt.Errorf("repository and token required")
	}
	return a, nil
}
func (a API) request(method, path string, body io.Reader, content string) (*http.Response, error) {
	target := a.Base + "/repos/" + a.Repository + path
	req, e := http.NewRequestWithContext(context.Background(), method, target, body)
	if e != nil {
		return nil, fmt.Errorf("invalid API request")
	}
	req.Header.Set("Authorization", "Bearer "+a.Token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	if content != "" {
		req.Header.Set("Content-Type", content)
	}
	r, e := a.Client.Do(req)
	if e != nil {
		return nil, fmt.Errorf("GitHub request failed")
	}
	return r, nil
}
func (a API) json(method, path string, body, out any) (int, error) {
	var reader io.Reader
	if body != nil {
		b, e := json.Marshal(body)
		if e != nil {
			return 0, e
		}
		reader = bytes.NewReader(b)
	}
	r, e := a.request(method, path, reader, "application/json")
	if e != nil {
		return 0, e
	}
	defer r.Body.Close()
	if r.StatusCode < 200 || r.StatusCode >= 300 {
		return r.StatusCode, fmt.Errorf("GitHub HTTP %d", r.StatusCode)
	}
	if out != nil {
		e = json.NewDecoder(io.LimitReader(r.Body, 16<<20)).Decode(out)
	}
	return r.StatusCode, e
}

type Run struct {
	ID         int64  `json:"id"`
	HeadSHA    string `json:"head_sha"`
	HeadBranch string `json:"head_branch"`
	Event      string `json:"event"`
	Status     string `json:"status"`
	Conclusion string `json:"conclusion"`
	Attempt    int    `json:"run_attempt"`
}

func (a API) RunAttempt(id, attempt string) (Run, error) {
	var r Run
	if !decimal.MatchString(id) || !decimal.MatchString(attempt) {
		return r, fmt.Errorf("invalid run attempt identity")
	}
	_, e := a.json("GET", "/actions/runs/"+id+"/attempts/"+attempt, nil, &r)
	return r, e
}

func qualifiedName(arch string, m Marker) string {
	return "qualified-" + arch + "-" + m.RunID + "-attempt-" + m.RunAttempt
}
func checkRun(r Run, m Marker, complete bool) error {
	if strconv.FormatInt(r.ID, 10) != m.RunID || r.HeadSHA != m.SourceSHA || r.HeadBranch != "main" || r.Event != "workflow_dispatch" || strconv.Itoa(r.Attempt) != m.RunAttempt {
		return fmt.Errorf("qualification run identity mismatch")
	}
	if complete && (r.Status != "completed" || r.Conclusion != "success") {
		return fmt.Errorf("preparation run did not succeed")
	}
	return nil
}

type Artifact struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	Digest      string `json:"digest"`
	Expired     bool   `json:"expired"`
	WorkflowRun struct {
		ID      int64  `json:"id"`
		HeadSHA string `json:"head_sha"`
	} `json:"workflow_run"`
}

func (a API) Artifacts(run string) ([]Artifact, error) {
	all := []Artifact{}
	for page := 1; page <= 1000; page++ {
		var batch struct {
			Artifacts []Artifact `json:"artifacts"`
		}
		_, e := a.json("GET", fmt.Sprintf("/actions/runs/%s/artifacts?per_page=100&page=%d", run, page), nil, &batch)
		if e != nil {
			return nil, e
		}
		all = append(all, batch.Artifacts...)
		if len(batch.Artifacts) < 100 {
			return all, nil
		}
	}
	return nil, fmt.Errorf("artifact pagination limit exceeded")
}
func (a API) Bind(m *Marker, q []Qualified) error {
	r, e := a.RunAttempt(m.RunID, m.RunAttempt)
	if e != nil {
		return e
	}
	if e = checkRun(r, *m, false); e != nil {
		return e
	}
	artifacts, e := a.Artifacts(m.RunID)
	if e != nil {
		return e
	}
	for _, v := range q {
		var match *Artifact
		for i := range artifacts {
			art := &artifacts[i]
			if art.Name == qualifiedName(v.Arch, *m) {
				if match != nil {
					return fmt.Errorf("ambiguous qualified artifact")
				}
				match = art
			}
		}
		if match == nil || match.Expired || match.WorkflowRun.HeadSHA != m.SourceSHA || strconv.FormatInt(match.WorkflowRun.ID, 10) != m.RunID {
			return fmt.Errorf("qualified artifact source mismatch or missing")
		}
		m.Artifacts = append(m.Artifacts, ArtifactBinding{match.ID, v.Arch, match.Digest, v.SHA256, v.Archive})
	}
	return ValidateMarker(*m)
}
func (a API) Download(path, destination string) error {
	r, e := a.request("GET", path, nil, "")
	if e != nil {
		return e
	}
	if r.StatusCode == 302 {
		location := r.Header.Get("Location")
		r.Body.Close()
		u, e := url.Parse(location)
		if e != nil || u.Scheme != "https" || u.User != nil {
			return fmt.Errorf("invalid artifact redirect")
		}
		req, e := http.NewRequest("GET", location, nil)
		if e != nil {
			return fmt.Errorf("invalid download request")
		}
		r, e = a.Client.Do(req)
		if e != nil {
			return fmt.Errorf("artifact download failed")
		}
	}
	defer r.Body.Close()
	if r.StatusCode != 200 {
		return fmt.Errorf("download HTTP %d", r.StatusCode)
	}
	f, e := os.OpenFile(destination, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if e != nil {
		return e
	}
	n, e := io.Copy(f, io.LimitReader(r.Body, maxDownload+1))
	ce := f.Close()
	if e != nil {
		return e
	}
	if ce != nil {
		return ce
	}
	if n > maxDownload {
		return fmt.Errorf("artifact exceeds size limit")
	}
	return nil
}

// Extract only regular receipt/archive files. Archives stay opaque for the tester.
func extract(zipPath, dir string) error {
	z, e := zip.OpenReader(zipPath)
	if e != nil {
		return e
	}
	defer z.Close()
	var total uint64
	count := 0
	for _, f := range z.File {
		clean := filepath.Clean(f.Name)
		if strings.Contains(f.Name, "\\") || filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, "../") {
			return fmt.Errorf("unsafe zip path")
		}
		if f.FileInfo().IsDir() {
			continue
		}
		if !f.Mode().IsRegular() || (!strings.HasSuffix(clean, ".json") && !strings.HasSuffix(clean, ".tar.gz")) {
			return fmt.Errorf("unexpected zip member")
		}
		if f.UncompressedSize64 > uint64(maxDownload) {
			return fmt.Errorf("zip member exceeds limit")
		}
		total += f.UncompressedSize64
		count++
		if total > uint64(maxDownload) || count > 4 {
			return fmt.Errorf("zip expansion limit exceeded")
		}
		target := filepath.Join(dir, clean)
		if e = os.MkdirAll(filepath.Dir(target), 0755); e != nil {
			return e
		}
		out, e := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
		if e != nil {
			return e
		}
		in, e := f.Open()
		if e != nil {
			out.Close()
			return e
		}
		n, e := io.Copy(out, io.LimitReader(in, int64(f.UncompressedSize64)+1))
		in.Close()
		ce := out.Close()
		if e != nil {
			return e
		}
		if ce != nil {
			return ce
		}
		if uint64(n) != f.UncompressedSize64 {
			return fmt.Errorf("zip size mismatch")
		}
	}
	return nil
}
func (a API) Fetch(m Marker, out string, wait bool) error {
	if e := ValidateMarker(m); e != nil {
		return e
	}
	deadline := time.Now().Add(8 * time.Minute)
	for {
		r, e := a.RunAttempt(m.RunID, m.RunAttempt)
		if e != nil {
			return e
		}
		if e = checkRun(r, m, false); e != nil {
			return e
		}
		if !wait {
			break
		}
		if r.Status == "completed" {
			if e = checkRun(r, m, true); e != nil {
				return e
			}
			break
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("timed out waiting for preparation run")
		}
		time.Sleep(5 * time.Second)
	}
	artifacts, e := a.Artifacts(m.RunID)
	if e != nil {
		return e
	}
	if _, e = os.Stat(out); e == nil {
		return fmt.Errorf("download output must not exist")
	}
	if e = os.MkdirAll(out, 0755); e != nil {
		return e
	}
	for _, b := range m.Artifacts {
		ok := false
		for _, ar := range artifacts {
			if ar.ID == b.ID && !ar.Expired && ar.Name == qualifiedName(b.Arch, m) && ar.Digest == b.Digest && ar.WorkflowRun.HeadSHA == m.SourceSHA && strconv.FormatInt(ar.WorkflowRun.ID, 10) == m.RunID {
				ok = true
			}
		}
		if !ok {
			return fmt.Errorf("bound artifact unavailable or changed")
		}
		tmp, e := os.CreateTemp("", "binary-artifact-*.zip")
		if e != nil {
			return e
		}
		name := tmp.Name()
		tmp.Close()
		os.Remove(name)
		e = a.Download(fmt.Sprintf("/actions/artifacts/%d/zip", b.ID), name)
		if e == nil {
			var sum string
			sum, e = Hash(name)
			if e == nil && "sha256:"+sum != b.Digest {
				e = fmt.Errorf("artifact zip digest mismatch")
			}
		}
		if e == nil {
			e = extract(name, filepath.Join(out, b.Arch))
		}
		os.Remove(name)
		if e != nil {
			return e
		}
	}
	q, e := Verify(out, m.SourceSHA, m.Version, m.RunID, m.RunAttempt)
	if e != nil {
		return e
	}
	return MatchBindings(q, m)
}
