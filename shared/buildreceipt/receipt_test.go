package buildreceipt

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tdesaules/github-actions/shared/catalog"
	"github.com/tdesaules/github-actions/shared/publication"
)

func TestUntestedReceiptAndTransfer(t *testing.T) {
	dir := t.TempDir()
	archive, receipt := filepath.Join(dir, "image.oci.tar"), filepath.Join(dir, "build.json")
	if err := os.WriteFile(archive, []byte("archive fixture"), 0644); err != nil {
		t.Fatal(err)
	}
	sum, err := publication.Checksum(archive)
	if err != nil {
		t.Fatal(err)
	}
	expected := Receipt{Image: "check-fixture", Arch: "amd64", Revision: strings.Repeat("a", 40), Version: "candidate"}
	if err = Write(archive, receipt, sum, expected); err != nil {
		t.Fatal(err)
	}
	if _, err = Verify(archive, receipt, expected); err != nil {
		t.Fatal(err)
	}
	var recorded Receipt
	if err = catalog.Read(receipt, &recorded); err != nil {
		t.Fatal(err)
	}
	if recorded.Tested {
		t.Fatal("build marked an untested archive qualified")
	}
	for _, mutate := range []func(*Receipt){func(r *Receipt) { r.Revision = strings.Repeat("b", 40) }, func(r *Receipt) { r.Arch = "arm64" }, func(r *Receipt) { r.Image = "other-fixture" }, func(r *Receipt) { r.Version = "other" }} {
		wrong := expected
		mutate(&wrong)
		if _, err := Verify(archive, receipt, wrong); err == nil {
			t.Fatal("wrong matrix context accepted")
		}
	}
	recorded.Tested = true
	if err = catalog.Write(receipt, recorded); err != nil {
		t.Fatal(err)
	}
	if _, err := Verify(archive, receipt, expected); err == nil {
		t.Fatal("qualification metadata accepted as a build receipt")
	}
	if err = Write(archive, receipt, sum, expected); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(archive, []byte("tampered archive"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := Verify(archive, receipt, expected); err == nil {
		t.Fatal("tampered archive accepted")
	}
	if Write(archive, receipt, sum, expected) == nil {
		t.Fatal("builder checksum mismatch accepted")
	}
}
