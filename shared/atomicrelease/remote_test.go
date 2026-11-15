package atomicrelease

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRemoteIntegrityPromotionAndRetry(t *testing.T) {
	e := evidenceFixture(t)
	index := manifest{MediaType: "application/vnd.oci.image.index.v1+json"}
	for _, r := range []Receipt{e.Qualified.AMD64, e.Qualified.ARM64} {
		d := descriptor{Digest: r.ManifestDigest}
		d.Platform.Architecture = r.Arch
		d.Platform.OS = "linux"
		index.Manifests = append(index.Manifests, d)
	}
	raw, _ := json.Marshal(index)
	e.Image = "ghcr.io/owner/os@" + fmtDigest(raw)
	iso := manifest{MediaType: "application/vnd.oci.image.manifest.v1+json"}
	for _, f := range e.ISOFiles {
		d := descriptor{Digest: "sha256:" + f.SHA256, MediaType: f.MediaType, Annotations: map[string]string{"org.opencontainers.image.title": f.Path}}
		if f.Path == e.ISOFile {
			d.Size = e.Qualified.ISO.Size
		}
		iso.Layers = append(iso.Layers, d)
	}
	isoRaw, _ := json.Marshal(iso)
	e.ISO = "ghcr.io/owner/os-iso@" + fmtDigest(isoRaw)
	channels := map[string][]byte{}
	failLatest := true
	posts := 0
	var release map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/token" {
			user, pass, ok := r.BasicAuth()
			if !ok || user != "actor" || pass != "registry-secret" {
				t.Error("registry credentials missing")
				w.WriteHeader(401)
				return
			}
			_, _ = w.Write([]byte(`{"token":"registry-bearer"}`))
			return
		}
		if strings.HasPrefix(r.URL.Path, "/v2/") {
			if r.Header.Get("Authorization") != "Bearer registry-bearer" {
				t.Error("registry bearer missing")
			}
			ref := r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:]
			if r.Method == "PUT" {
				if ref == "latest" && failLatest {
					w.WriteHeader(503)
					return
				}
				var m manifest
				if err := json.NewDecoder(r.Body).Decode(&m); err != nil {
					t.Fatal(err)
				}
				channels[ref], _ = json.Marshal(m)
				w.WriteHeader(201)
				return
			}
			switch {
			case ref == "45" || ref == "latest":
				_, _ = w.Write(channels[ref])
			case strings.Contains(r.URL.Path, "os-iso/"):
				_, _ = w.Write(isoRaw)
			default:
				_, _ = w.Write(raw)
			}
			return
		}
		if r.Header.Get("Authorization") != "Bearer app-secret" {
			t.Error("API authorization missing")
		}
		switch {
		case r.URL.Path == "/installation/repositories":
			_, _ = w.Write([]byte(`{"repositories":[{"full_name":"owner/os"}]}`))
		case r.Method == "GET":
			if release == nil {
				w.WriteHeader(404)
				return
			}
			_ = json.NewEncoder(w).Encode(release)
		case r.Method == "POST":
			posts++
			if err := json.NewDecoder(r.Body).Decode(&release); err != nil {
				t.Fatal(err)
			}
			release["html_url"] = "https://github.com/owner/os/releases/tag/1.2.3"
			w.WriteHeader(201)
			_ = json.NewEncoder(w).Encode(release)
		default:
			w.WriteHeader(500)
		}
	}))
	defer server.Close()
	a, err := NewRemote("owner/os", "app-secret", "registry-secret", "actor")
	if err != nil {
		t.Fatal(err)
	}
	a.Client = server.Client()
	a.APIURL = server.URL
	a.RegistryURL = server.URL
	if err = a.VerifyAppToken(); err != nil {
		t.Fatal(err)
	}
	verified, err := a.VerifyPublished(e)
	if err != nil {
		t.Fatal(err)
	}
	if err = a.Promote(verified); err == nil {
		t.Fatal("partial promotion failure not blocking")
	}
	failLatest = false
	if err = a.Promote(verified); err != nil {
		t.Fatal(err)
	}
	if fmtDigest(channels["45"]) != fmtDigest(raw) || fmtDigest(channels["latest"]) != fmtDigest(raw) {
		t.Fatal("promotion changed index")
	}
	final := strings.Repeat("f", 40)
	first, err := a.Release(e, final, "notes", true)
	if err != nil {
		t.Fatal(err)
	}
	second, err := a.Release(e, final, "notes", true)
	if err != nil {
		t.Fatal(err)
	}
	if first != second || posts != 1 {
		t.Fatal("retry created duplicate release")
	}
	if _, err = a.Release(e, final, "different notes", true); err == nil {
		t.Fatal("conflicting release overwritten")
	}
	release = nil
	if _, err = a.Release(e, final, "maintenance notes", false); err != nil {
		t.Fatal(err)
	}
	if release["make_latest"] != "false" {
		t.Fatal("older release replaced GitHub latest")
	}
	// Retrieval must verify digest, not trust registry response headers.
	raw = []byte(`{"wrong":"manifest"}`)
	if _, err = a.VerifyPublished(e); err == nil {
		t.Fatal("registry byte tampering accepted")
	}
}
func TestInstallationTokenFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(403) }))
	defer server.Close()
	a, _ := NewRemote("owner/os", "not-installation", "", "")
	a.APIURL = server.URL
	a.Client = server.Client()
	if err := a.VerifyAppToken(); err == nil {
		t.Fatal("unverified App token accepted")
	}
}
func TestManifestDigest(t *testing.T) {
	if got := fmtDigest([]byte("test")); got != "sha256:9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08" {
		t.Fatal(fmt.Sprint(got))
	}
}
