// Package buildreceipt keeps untested build receipts distinct from qualification metadata.
package buildreceipt

import (
	"fmt"
	"regexp"

	"github.com/tdesaules/github-actions/shared/catalog"
	"github.com/tdesaules/github-actions/shared/publication"
)

type Receipt struct {
	Image    string `json:"image"`
	Arch     string `json:"arch"`
	Revision string `json:"revision"`
	Version  string `json:"version"`
	SHA256   string `json:"archive_sha256"`
	Tested   bool   `json:"tested"`
}

func (r Receipt) Validate() error {
	if !regexp.MustCompile(`^[a-z0-9][a-z0-9-]+$`).MatchString(r.Image) || (r.Arch != "amd64" && r.Arch != "arm64") || !regexp.MustCompile(`^[a-f0-9]{40}$`).MatchString(r.Revision) || r.Version == "" || !regexp.MustCompile(`^[a-f0-9]{64}$`).MatchString(r.SHA256) || r.Tested {
		return fmt.Errorf("invalid untested build receipt")
	}
	return nil
}

func Write(archive, output, expected string, r Receipt) error {
	sum, err := publication.Checksum(archive)
	if err != nil {
		return err
	}
	if sum != expected {
		return fmt.Errorf("archive differs from builder checksum")
	}
	r.SHA256 = sum
	if err = r.Validate(); err != nil {
		return err
	}
	return catalog.Write(output, r)
}

func Verify(archive, receipt string, expected Receipt) (string, error) {
	var actual Receipt
	if err := catalog.Read(receipt, &actual); err != nil {
		return "", err
	}
	if err := actual.Validate(); err != nil {
		return "", err
	}
	if actual.Image != expected.Image || actual.Arch != expected.Arch || actual.Revision != expected.Revision || actual.Version != expected.Version {
		return "", fmt.Errorf("build receipt does not match this matrix job and source revision")
	}
	sum, err := publication.Checksum(archive)
	if err != nil {
		return "", err
	}
	if sum != actual.SHA256 {
		return "", fmt.Errorf("downloaded archive checksum mismatch")
	}
	return sum, nil
}
