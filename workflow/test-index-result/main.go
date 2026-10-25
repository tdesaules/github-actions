package main

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/tdesaules/github-actions/shared/catalog"
	"github.com/tdesaules/github-actions/shared/publication"
	"github.com/tdesaules/github-actions/shared/workflow"
)

func run() error {
	directory := flag.String("directory", workflow.Default("INPUT_DIRECTORY", ".build/fixtures"), "fixture directory")
	index := flag.String("index", os.Getenv("INPUT_INDEX"), "native index output")
	retry := flag.String("retry-index", os.Getenv("INPUT_RETRY-INDEX"), "retry output")
	flag.Parse()
	if flag.NArg() != 0 || *index == "" || *retry == "" {
		return fmt.Errorf("index and retry-index are required")
	}
	a, err := os.ReadFile(*index)
	if err != nil {
		return err
	}
	b, err := os.ReadFile(*retry)
	if err != nil {
		return err
	}
	if !bytes.Equal(a, b) {
		return fmt.Errorf("retry changed the index")
	}
	children, err := publication.Children(a)
	if err != nil {
		return err
	}
	var checksums map[string]string
	if err = catalog.Read(filepath.Join(*directory, "fixture-checksums.json"), &checksums); err != nil {
		return err
	}
	for arch, name := range map[string]string{"amd64": "fixture.oci.tar", "arm64": "arm64.oci.tar"} {
		archive := filepath.Join(*directory, name)
		sum, err := publication.Checksum(archive)
		if err != nil {
			return err
		}
		if sum != checksums[name] {
			return fmt.Errorf("index publication changed an archive")
		}
		digest, err := publication.ArchiveDigest(archive)
		if err != nil {
			return err
		}
		if digest != children[arch] {
			return fmt.Errorf("index changed the tested platform digest")
		}
	}
	cert, err := os.ReadFile(filepath.Join(*directory, "registry-cert.pem"))
	if err != nil {
		return err
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(cert) {
		return fmt.Errorf("invalid registry CA")
	}
	client := &http.Client{Timeout: 30 * time.Second, Transport: &http.Transport{TLSClientConfig: &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12}}, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}
	get := func(path string) ([]byte, int, error) {
		request, err := http.NewRequest("GET", "https://test-registry:5443/v2/fixture/"+path, nil)
		if err != nil {
			return nil, 0, err
		}
		request.Header.Set("Accept", "application/vnd.oci.image.index.v1+json,application/vnd.docker.distribution.manifest.list.v2+json")
		response, err := client.Do(request)
		if err != nil {
			return nil, 0, err
		}
		defer response.Body.Close()
		data, err := io.ReadAll(io.LimitReader(response.Body, 2<<20))
		return data, response.StatusCode, err
	}
	remote, status, err := get("manifests/" + publication.Hash(a))
	if err != nil {
		return err
	}
	if status != 200 || !bytes.Equal(remote, a) {
		return fmt.Errorf("remote index differs from tested output")
	}
	data, status, err := get("tags/list")
	if err != nil {
		return err
	}
	if status == 200 {
		var tags struct {
			Tags []string `json:"tags"`
		}
		if err = json.Unmarshal(data, &tags); err != nil {
			return err
		}
		if len(tags.Tags) != 0 {
			return fmt.Errorf("digest-only index publication created tags")
		}
	} else if status == 404 {
		var response struct {
			Errors []struct {
				Code string `json:"code"`
			} `json:"errors"`
		}
		if err = json.Unmarshal(data, &response); err != nil {
			return err
		}
		if len(response.Errors) == 0 || response.Errors[0].Code != "NAME_UNKNOWN" {
			return fmt.Errorf("unexpected registry error")
		}
	} else {
		return fmt.Errorf("registry tags request returned HTTP %d", status)
	}
	return workflow.Output("verified", true)
}
func main() { workflow.Main("test-index-result 1", run) }
