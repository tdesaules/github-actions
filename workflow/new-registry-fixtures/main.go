package main

import (
	"crypto/tls"
	"crypto/x509"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"github.com/tdesaules/github-actions/shared/catalog"
	"github.com/tdesaules/github-actions/shared/publication"
	"github.com/tdesaules/github-actions/shared/workflow"
)

func run() error {
	directory := flag.String("directory", workflow.Default("INPUT_DIRECTORY", ".build/fixtures"), "fixture directory")
	seed := flag.Bool("seed", os.Getenv("INPUT_SEED") == "true", "publish fixture before signature tests")
	sign := flag.Bool("signed-fixture", os.Getenv("INPUT_SIGNED-FIXTURE") == "true", "sign fixture before verification tests")
	flag.Parse()
	if flag.NArg() != 0 {
		return fmt.Errorf("unexpected arguments")
	}
	if err := os.MkdirAll(*directory, 0755); err != nil {
		return err
	}
	client := &http.Client{Timeout: 2 * time.Second, Transport: &http.Transport{}, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}
	var cert []byte
	var last error
	for i := 0; i < 60; i++ {
		response, err := client.Get("http://test-registry:5000/ca.pem")
		if err == nil {
			if response.StatusCode == 200 {
				cert, err = io.ReadAll(io.LimitReader(response.Body, 64<<10))
			} else {
				err = fmt.Errorf("registry certificate endpoint returned HTTP %d", response.StatusCode)
			}
			response.Body.Close()
		}
		if err == nil && len(cert) > 0 {
			break
		}
		last = err
		time.Sleep(time.Second)
	}
	if len(cert) == 0 {
		return fmt.Errorf("test registry unavailable: %v", last)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(cert) {
		return fmt.Errorf("test registry certificate is invalid")
	}
	client.Transport = &http.Transport{TLSClientConfig: &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12}}
	ready := false
	for i := 0; i < 60; i++ {
		response, err := client.Get("https://test-registry:5443/v2/")
		if err == nil {
			ready = response.StatusCode == 200
			response.Body.Close()
		}
		if ready {
			break
		}
		last = err
		time.Sleep(time.Second)
	}
	if !ready {
		return fmt.Errorf("TLS test registry is not ready: %v", last)
	}
	ca, err := filepath.Abs(filepath.Join(*directory, "registry-cert.pem"))
	if err != nil {
		return err
	}
	if err = os.WriteFile(ca, cert, 0644); err != nil {
		return err
	}
	os.Setenv("SSL_CERT_FILE", ca)
	os.Setenv("COSIGN_PASSWORD", "")
	os.Setenv("NO_PROXY", "test-registry")
	os.Setenv("no_proxy", "test-registry")
	archive := filepath.Join(*directory, "fixture.oci.tar")
	digest, err := publication.ArchiveDigest(archive)
	if err != nil {
		return err
	}
	repository := "test-registry:5443/fixture"
	reference := repository + "@" + digest
	if *seed || *sign {
		if err = catalog.Run("skopeo", "copy", "--retry-times=3", "--preserve-digests", "oci-archive:"+archive, "docker://"+reference); err != nil {
			return err
		}
	}
	prefix := filepath.Join(*directory, "cosign")
	if err = catalog.Run("cosign", "generate-key-pair", "--output-key-prefix", prefix); err != nil {
		return err
	}
	if *sign {
		private, err := os.MkdirTemp("", "fixture-signing-")
		if err != nil {
			return err
		}
		defer os.RemoveAll(private)
		config := filepath.Join(private, "signing.json")
		if err = catalog.Run("cosign", "signing-config", "create", "--out", config); err != nil {
			return err
		}
		if err = catalog.Run("cosign", "sign", "--yes", "--key", prefix+".key", "--signing-config", config, reference); err != nil {
			return err
		}
	}
	owner := os.Getenv("GITHUB_OUTPUT")
	if owner == "" {
		owner = "."
	}
	info, err := os.Stat(owner)
	if err != nil {
		return err
	}
	stat := info.Sys().(*syscall.Stat_t)
	for _, path := range []string{prefix + ".key", prefix + ".pub"} {
		if err = os.Chown(path, int(stat.Uid), int(stat.Gid)); err != nil {
			return err
		}
	}
	for name, value := range map[string]string{"repository": "docker://" + repository, "reference": reference, "destination": "docker://" + reference} {
		if err = workflow.Output(name, value); err != nil {
			return err
		}
	}
	return nil
}
func main() { workflow.Main("new-registry-fixtures 1", run) }
