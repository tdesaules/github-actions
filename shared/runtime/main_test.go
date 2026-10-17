package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestOIDCAudienceAndBearer(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("audience") != "sigstore" || r.Header.Get("Authorization") != "Bearer request-secret" {
			t.Error("OIDC audience or request bearer was not passed correctly")
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_, _ = w.Write([]byte(`{"value":"test-identity-token"}`))
	}))
	defer server.Close()
	old := http.DefaultTransport
	http.DefaultTransport = server.Client().Transport
	defer func() { http.DefaultTransport = old }()
	t.Setenv("ACTIONS_ID_TOKEN_REQUEST_URL", server.URL+"?audience=wrong")
	t.Setenv("ACTIONS_ID_TOKEN_REQUEST_TOKEN", "request-secret")
	token, err := oidcToken()
	if err != nil || token != "test-identity-token" {
		t.Fatal(token, err)
	}
}

func TestOIDCRejectsPlainHTTP(t *testing.T) {
	t.Setenv("ACTIONS_ID_TOKEN_REQUEST_URL", "http://example.invalid/token")
	t.Setenv("ACTIONS_ID_TOKEN_REQUEST_TOKEN", "request-secret")
	if _, err := oidcToken(); err == nil {
		t.Fatal("unencrypted OIDC request accepted")
	}
}

func TestBuildArgumentsPreserveSpacesAndNoEvaluation(t *testing.T) {
	t.Setenv("INPUT_IMAGE", "localhost/example")
	t.Setenv("INPUT_VERSION", "0.0.0-bootstrap")
	t.Setenv("INPUT_CONTEXT", "fixture with spaces")
	t.Setenv("INPUT_LABELS", `{"example":"$(touch /tmp/should-not-exist)","a":"two words"}`)
	args, err := buildArgs()
	if err != nil {
		t.Fatal(err)
	}
	if args[len(args)-1] != "fixture with spaces" {
		t.Fatal(args)
	}
	joined := strings.Join(args, "\x00")
	if !strings.Contains(joined, "--label\x00example=$(touch /tmp/should-not-exist)") {
		t.Fatal(args)
	}
}

func TestInvalidBuildInputs(t *testing.T) {
	t.Setenv("INPUT_IMAGE", "example")
	t.Setenv("INPUT_VERSION", "1.0.0")
	t.Setenv("INPUT_PLATFORM", "linux/386")
	if _, err := buildArgs(); err == nil {
		t.Fatal("unsupported platform accepted")
	}
	t.Setenv("INPUT_PLATFORM", "linux/arm64")
	t.Setenv("INPUT_BUILD-ARGS", `["bad"]`)
	if _, err := buildArgs(); err == nil {
		t.Fatal("invalid JSON object accepted")
	}
}

func TestJSONMapDeterministic(t *testing.T) {
	t.Setenv("INPUT_LABELS", `{"z":"last","a":"first"}`)
	got, err := jsonMap("labels")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, []string{"a=first", "z=last"}) {
		t.Fatal(got)
	}
}

func TestOutputProtocol(t *testing.T) {
	p := filepath.Join(t.TempDir(), "outputs with spaces")
	if err := os.WriteFile(p, nil, 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GITHUB_OUTPUT", p)
	if err := output("digest", "sha256:example"); err != nil {
		t.Fatal(err)
	}
	if err := output("injection", "a\nb=c"); err == nil {
		t.Fatal("output injection accepted")
	}
	b, _ := os.ReadFile(p)
	if string(b) != "digest=sha256:example\n" {
		t.Fatal(string(b))
	}
}

func TestSignatureRejectsMutableReference(t *testing.T) {
	t.Setenv("INPUT_IMAGE", "ghcr.io/example/image:latest")
	if err := signature("test-image-signature"); err == nil {
		t.Fatal("mutable reference accepted")
	}
}

func TestOIDCRequiresCredentials(t *testing.T) {
	t.Setenv("ACTIONS_ID_TOKEN_REQUEST_URL", "")
	t.Setenv("ACTIONS_ID_TOKEN_REQUEST_TOKEN", "")
	if _, err := oidcToken(); err == nil {
		t.Fatal("missing OIDC credentials accepted")
	}
}

func TestOperationCannotEscapeTool(t *testing.T) {
	old := tool
	tool = "syft"
	defer func() { tool = old }()
	if err := execute("build-image"); err == nil {
		t.Fatal("wrong tool accepted")
	}
}

func TestKeylessCannotDisableTransparency(t *testing.T) {
	t.Setenv("INPUT_IMAGE", "registry/image@sha256:"+strings.Repeat("0", 64))
	t.Setenv("INPUT_TRANSPARENCY-LOG", "false")
	if err := signature("new-image-signature"); err == nil || !strings.Contains(err.Error(), "explicit key") {
		t.Fatal("keyless transparency bypass accepted", err)
	}
}

func TestCanonicalReferenceKeepsRegistryPort(t *testing.T) {
	for _, value := range []string{"registry:5000/team/image:tag", "docker://registry:5000/team/image:tag@sha256:abc", "registry:5000/team/image"} {
		if got := repositoryName(value); got != "registry:5000/team/image" {
			t.Fatal(got)
		}
	}
}
