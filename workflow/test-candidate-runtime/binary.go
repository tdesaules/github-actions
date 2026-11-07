package main

import (
	"archive/zip"
	"bufio"
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/tdesaules/github-actions/shared/candidate"
	"github.com/tdesaules/github-actions/shared/catalog"
	"github.com/tdesaules/github-actions/shared/fixtures"
)

// githubFixture is an isolated HTTPS proxy: it terminates fixture TLS locally and
// never forwards a request. This tests the production fixed GitHub endpoints with
// an ephemeral trust root and dummy token, without changing the public interface.
type githubFixture struct {
	mu                sync.Mutex
	source            string
	artifacts         []map[string]any
	zips              map[string][]byte
	uploads           map[string][]byte
	released, corrupt bool
}

func (f *githubFixture) serve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if r.Header.Get("Authorization") != "Bearer unused-fixture-token" {
		http.Error(w, "fixture authorization missing", 401)
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/repos/fixture/repo")
	var value any
	switch {
	case path == "/actions/runs/123":
		value = map[string]any{"id": 123, "head_sha": f.source, "head_branch": "main", "event": "workflow_dispatch", "status": "completed", "conclusion": "success", "run_attempt": 1}
	case path == "/actions/runs/123/artifacts":
		value = map[string]any{"artifacts": f.artifacts}
	case strings.HasPrefix(path, "/actions/artifacts/") && strings.HasSuffix(path, "/zip"):
		data, ok := f.zips[path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		if f.corrupt {
			data = []byte("corrupted artifact")
		}
		_, _ = w.Write(data)
		return
	case path == "/releases" && r.Method == "GET":
		value = []any{}
	case path == "/releases/tags/1.0.0" && !f.released:
		http.NotFound(w, r)
		return
	case path == "/releases" && r.Method == "POST", path == "/releases/7" && r.Method == "PATCH", path == "/releases/tags/1.0.0":
		f.released = true
		value = map[string]any{"id": 7, "tag_name": "1.0.0", "draft": false, "prerelease": false}
	case path == "/releases/7/assets" && r.Method == "GET":
		assets := []map[string]any{}
		for name, data := range f.uploads {
			assets = append(assets, map[string]any{"id": len(assets) + 1, "name": name, "size": len(data), "digest": fmt.Sprintf("sha256:%x", sha256.Sum256(data))})
		}
		value = assets
	case path == "/releases/7/assets" && r.Method == "POST":
		name := r.URL.Query().Get("name")
		if _, exists := f.uploads[name]; exists {
			http.Error(w, "duplicate upload", 409)
			return
		}
		data, err := io.ReadAll(io.LimitReader(r.Body, 4<<20))
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		f.uploads[name] = data
		w.WriteHeader(201)
		_, _ = w.Write([]byte("{}"))
		return
	default:
		http.Error(w, "unexpected fixture API operation: "+r.Method+" "+path, 400)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(value)
}

func (f *githubFixture) proxy() (*httptest.Server, error) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, err
	}
	cert := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "offline candidate fixture"}, DNSNames: []string{"api.github.com", "uploads.github.com"}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour), IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
	der, err := x509.CreateCertificate(rand.Reader, cert, cert, &key.PublicKey, key)
	if err != nil {
		return nil, err
	}
	pemCert := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	if err := os.WriteFile("/reports/fixture-ca.pem", pemCert, 0644); err != nil {
		return nil, err
	}
	tlsCert := tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "CONNECT" || (r.Host != "api.github.com:443" && r.Host != "uploads.github.com:443") {
			http.Error(w, "fixture proxy refuses external destination", 403)
			return
		}
		conn, _, err := w.(http.Hijacker).Hijack()
		if err != nil {
			return
		}
		defer conn.Close()
		_, _ = io.WriteString(conn, "HTTP/1.1 200 Connection Established\r\n\r\n")
		secure := tls.Server(conn, &tls.Config{Certificates: []tls.Certificate{tlsCert}, MinVersion: tls.VersionTLS12})
		if err := secure.Handshake(); err != nil {
			return
		}
		reader := bufio.NewReader(secure)
		for {
			req, err := http.ReadRequest(reader)
			if err != nil {
				return
			}
			rec := httptest.NewRecorder()
			f.serve(rec, req)
			req.Body.Close()
			response := rec.Result()
			response.ContentLength = int64(rec.Body.Len())
			err = response.Write(secure)
			response.Body.Close()
			if err != nil || req.Close {
				return
			}
		}
	}))
	return server, nil
}

func fixtureGit(directory string, args ...string) (string, error) {
	cmd := exec.Command("git", append([]string{"-c", "safe.directory=" + directory}, args...)...)
	cmd.Dir = directory
	cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_AUTHOR_NAME=Fixture", "GIT_AUTHOR_EMAIL=fixture@example.invalid", "GIT_COMMITTER_NAME=Fixture", "GIT_COMMITTER_EMAIL=fixture@example.invalid", "GIT_AUTHOR_DATE=2026-01-02T00:00:00Z", "GIT_COMMITTER_DATE=2026-01-02T00:00:00Z")
	b, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("fixture git: %w: %s", err, b)
	}
	return strings.TrimSpace(string(b)), nil
}

func binaryCases(root *candidate.Root, image string, results *[]result) error {
	directory := "/reports/binary-source"
	source, err := fixtures.RustRepository(directory)
	if err != nil {
		return err
	}
	f := &githubFixture{source: source, zips: map[string][]byte{}, uploads: map[string][]byte{}}
	bindings := []map[string]any{}
	originals := map[string][]byte{}
	for i, arch := range []string{"amd64", "arm64"} {
		name := "niri-tonic-files-1.0.0-linux-" + arch + ".tar.gz"
		data := []byte("opaque exact qualified fixture archive " + arch)
		originals[name] = data
		sum := fmt.Sprintf("%x", sha256.Sum256(data))
		receipt := map[string]any{"schema_version": 1, "component": "niri-tonic-files", "source_sha": source, "version": "1.0.0", "arch": arch, "archive": name, "sha256": sum, "run_id": "123", "run_attempt": "1", "tested": true}
		path := filepath.Join("/reports/binary-qualified", arch)
		if err := os.MkdirAll(path, 0755); err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(path, name), data, 0644); err != nil {
			return err
		}
		if err := catalog.Write(filepath.Join(path, name+".receipt.json"), receipt); err != nil {
			return err
		}
		raw, _ := json.Marshal(receipt)
		var zipped bytes.Buffer
		z := zip.NewWriter(&zipped)
		for n, b := range map[string][]byte{name: data, name + ".receipt.json": raw} {
			w, err := z.Create(n)
			if err != nil {
				return err
			}
			if _, err = w.Write(b); err != nil {
				return err
			}
		}
		if err := z.Close(); err != nil {
			return err
		}
		id := i + 1
		digest := fmt.Sprintf("sha256:%x", sha256.Sum256(zipped.Bytes()))
		f.zips[fmt.Sprintf("/actions/artifacts/%d/zip", id)] = zipped.Bytes()
		f.artifacts = append(f.artifacts, map[string]any{"id": id, "name": "qualified-" + arch + "-123", "digest": digest, "expired": false, "workflow_run": map[string]any{"id": 123, "head_sha": source}})
		bindings = append(bindings, map[string]any{"id": id, "arch": arch, "digest": digest, "sha256": sum, "archive": name})
	}
	marker := map[string]any{"schema_version": 1, "source_sha": source, "version": "1.0.0", "run_id": "123", "run_attempt": "1", "artifacts": bindings}
	plan := map[string]any{"mode": "prepare", "source_sha": source, "version": "1.0.0", "has_release": true}
	if err := catalog.Write("/reports/binary-plan.json", plan); err != nil {
		return err
	}
	cliff := "[changelog]\nheader = \"# Changelog\\n\\n## [Unreleased]\\n\"\nbody = \"\\n## [{{ version }}]\\n\\n### Added\\n\\n- Fixture release.\\n\"\n[git]\nconventional_commits = false\nfilter_unconventional = false\n"
	if err := os.WriteFile(filepath.Join(directory, "cliff.toml"), []byte(cliff), 0644); err != nil {
		return err
	}
	if image == "workflow-read-binary-release" || image == "workflow-publish-binary-release" {
		for _, name := range []string{"Cargo.toml", "Cargo.lock"} {
			p := filepath.Join(directory, name)
			b, err := os.ReadFile(p)
			if err != nil {
				return err
			}
			if err = os.WriteFile(p, bytes.ReplaceAll(b, []byte(`version = "0.1.0"`), []byte(`version = "1.0.0"`)), 0644); err != nil {
				return err
			}
		}
		if err := os.WriteFile(filepath.Join(directory, "CHANGELOG.md"), []byte("# Changelog\n\n## [Unreleased]\n\n## [1.0.0]\n\n### Added\n\n- Fixture.\n"), 0644); err != nil {
			return err
		}
		if _, err := fixtureGit(directory, "add", "Cargo.toml", "Cargo.lock", "CHANGELOG.md"); err != nil {
			return err
		}
		if _, err := fixtureGit(directory, "commit", "-m", "chore(release): prepare 1.0.0"); err != nil {
			return err
		}
		raw, _ := json.Marshal(marker)
		if _, err := fixtureGit(directory, "tag", "-a", "1.0.0", "-m", "binary-release-v1\n"+string(raw)); err != nil {
			return err
		}
	}
	if _, err := fixtureGit("/reports", "clone", "--bare", directory, "/reports/binary-origin.git"); err != nil {
		return err
	}
	if err := os.WriteFile("/reports/binary-outputs", nil, 0666); err != nil {
		return err
	}
	if err := os.Chmod("/reports/binary-outputs", 0666); err != nil {
		return err
	}
	// Git's local transport is restricted to this disposable fixture remote.
	env := []string{"GITHUB_RUN_ID=123", "GITHUB_RUN_ATTEMPT=1", "GITHUB_OUTPUT=/reports/binary-outputs", "GITHUB_REF=refs/heads/main", "GITHUB_EVENT_NAME=workflow_dispatch", "INPUT_SOURCE_SHA=" + source, "INPUT_REPOSITORY=fixture/repo", "INPUT_TOKEN=unused-fixture-token", "INPUT_PLAN=/reports/binary-plan.json", "INPUT_QUALIFIED_DIRECTORY=/reports/binary-qualified", "INPUT_TAG=1.0.0", "INPUT_OUTPUT=/reports/binary-downloaded", "GIT_CONFIG_COUNT=2", "GIT_CONFIG_KEY_0=safe.directory", "GIT_CONFIG_VALUE_0=" + directory, "GIT_CONFIG_KEY_1=url.file:///reports/binary-origin.git.insteadOf", "GIT_CONFIG_VALUE_1=https://github.com/fixture/repo.git"}
	if err := filepath.WalkDir("/reports", func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		return os.Chown(path, 65532, 65532)
	}); err != nil {
		return err
	}
	server, err := f.proxy()
	if err != nil {
		return err
	}
	defer server.Close()
	env = append(env, "HTTPS_PROXY="+server.URL, "HTTP_PROXY="+server.URL, "NO_PROXY=", "SSL_CERT_FILE=/reports/fixture-ca.pem", "SSL_CERT_DIR=/reports/no-certs")
	run := func(name string, extra []string, success bool, diagnostic string) error {
		return actionCase(root, results, name, directory, nil, append(append([]string{}, env...), extra...), success, diagnostic)
	}
	switch image {
	case "workflow-plan-rust-gtk-release":
		if err := run("native empty Rust release plan", []string{"INPUT_EVENT=push", "INPUT_REF=refs/heads/main", "INPUT_BEFORE=" + source, "INPUT_OUTPUT=/reports/rust-plan.json"}, true, ""); err != nil {
			return err
		}
		var p struct {
			Mode     string
			HasTests bool   `json:"has_tests"`
			Source   string `json:"source_sha"`
		}
		if err := catalog.Read("/reports/rust-plan.json", &p); err != nil {
			return err
		}
		if p.Mode != "development" || p.HasTests || p.Source != source {
			return fmt.Errorf("invalid empty Rust plan")
		}
		return run("reject prefixed release tag", []string{"INPUT_EVENT=push", "INPUT_REF=refs/tags/v1.0.0"}, false, "stable no-v")
	case "workflow-prepare-binary-release":
		if err := run("prepare release against isolated Git and artifact service", nil, true, ""); err != nil {
			return err
		}
		ref, err := fixtureGit("/reports", "--git-dir=/reports/binary-origin.git", "rev-parse", "refs/tags/1.0.0^{commit}")
		if err != nil || ref == source {
			return fmt.Errorf("preparation failed to push metadata commit and annotated tag: %v", err)
		}
		return run("reject stale release source after preparation", nil, false, "source SHA")
	case "workflow-read-binary-release":
		if err := run("read bound exact binary archives", nil, true, ""); err != nil {
			return err
		}
		for _, arch := range []string{"amd64", "arm64"} {
			name := "niri-tonic-files-1.0.0-linux-" + arch + ".tar.gz"
			b, err := os.ReadFile(filepath.Join("/reports/binary-downloaded", arch, name))
			if err != nil || !bytes.Equal(b, originals[name]) {
				return fmt.Errorf("download changed exact archive bytes: %v", err)
			}
		}
		f.mu.Lock()
		f.corrupt = true
		f.mu.Unlock()
		return run("reject changed artifact ZIP bytes", []string{"INPUT_OUTPUT=/reports/rejected-download"}, false, "zip digest mismatch")
	case "workflow-publish-binary-release":
		if err := run("publish exact bound binary assets to isolated service", nil, true, ""); err != nil {
			return err
		}
		if err := run("retry exact binary asset publication", nil, true, ""); err != nil {
			return err
		}
		f.mu.Lock()
		for name, b := range originals {
			if !bytes.Equal(b, f.uploads[name]) {
				f.mu.Unlock()
				return fmt.Errorf("publication changed binary bytes")
			}
		}
		count := len(f.uploads)
		f.mu.Unlock()
		if count != 4 {
			return fmt.Errorf("expected two binary archives, SHA256SUMS and release evidence; got %d", count)
		}
		path := "/reports/binary-qualified/amd64/niri-tonic-files-1.0.0-linux-amd64.tar.gz"
		if err := os.WriteFile(path, []byte("corrupted"), 0644); err != nil {
			return err
		}
		return run("reject changed qualified archive before publication", nil, false, "checksum mismatch")
	}
	return fmt.Errorf("unhandled binary runtime %s", image)
}
