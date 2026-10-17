package runtimeaction

import (
	"archive/tar"
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func archiveFixture(t *testing.T, arch string, corrupt bool) (string, string) {
	t.Helper()
	blobs := map[string][]byte{}
	blob := func(data []byte, kind string) ociDescriptor {
		digest := fmt.Sprintf("sha256:%x", sha256.Sum256(data))
		blobs["blobs/sha256/"+digest[7:]] = data
		return ociDescriptor{MediaType: kind, Digest: digest, Size: int64(len(data))}
	}
	config := blob([]byte(fmt.Sprintf(`{"architecture":%q,"os":"linux"}`, arch)), "application/vnd.oci.image.config.v1+json")
	layer := blob([]byte("layer content"), "application/vnd.oci.image.layer.v1.tar")
	manifest, _ := json.Marshal(struct {
		SchemaVersion int             `json:"schemaVersion"`
		Config        ociDescriptor   `json:"config"`
		Layers        []ociDescriptor `json:"layers"`
	}{2, config, []ociDescriptor{layer}})
	d := blob(manifest, "application/vnd.oci.image.manifest.v1+json")
	blobs["index.json"], _ = json.Marshal(ociIndex{SchemaVersion: 2, Manifests: []ociDescriptor{d}})
	if corrupt {
		blobs["blobs/sha256/"+layer.Digest[7:]] = []byte("corrupt")
	}
	path := filepath.Join(t.TempDir(), "archive with spaces.tar")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	w := tar.NewWriter(f)
	for name, data := range blobs {
		if err = w.WriteHeader(&tar.Header{Name: name, Size: int64(len(data)), Mode: 0644, Typeflag: tar.TypeReg}); err != nil {
			t.Fatal(err)
		}
		if _, err = w.Write(data); err != nil {
			t.Fatal(err)
		}
	}
	if err = w.Close(); err != nil {
		t.Fatal(err)
	}
	if err = f.Close(); err != nil {
		t.Fatal(err)
	}
	return path, d.Digest
}

func TestIndexPreservesDigestsAndIsOrderIndependent(t *testing.T) {
	amd, amdDigest := archiveFixture(t, "amd64", false)
	arm, armDigest := archiveFixture(t, "arm64", false)
	first, err := assembleIndex([]string{amd, arm}, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	second, err := assembleIndex([]string{arm, amd}, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first, second) {
		t.Fatal("input order changed index digest")
	}
	var index ociIndex
	if err = json.Unmarshal(first, &index); err != nil {
		t.Fatal(err)
	}
	if index.Manifests[0].Digest != amdDigest || index.Manifests[1].Digest != armDigest {
		t.Fatal("index changed tested child digests")
	}
}

func TestIndexRejectsCorruptionAndInvalidArchitectures(t *testing.T) {
	amd, _ := archiveFixture(t, "amd64", false)
	for _, fixture := range []struct {
		arch    string
		corrupt bool
	}{{"amd64", false}, {"386", false}, {"arm64", true}} {
		other, _ := archiveFixture(t, fixture.arch, fixture.corrupt)
		if _, err := assembleIndex([]string{amd, other}, t.TempDir()); err == nil {
			t.Fatalf("accepted invalid fixture: %+v", fixture)
		}
	}
}

func TestArgumentsOverrideEnvironmentAndRejectDispatch(t *testing.T) {
	t.Cleanup(func() { arguments = map[string]string{} })
	t.Setenv("INPUT_CONTEXT", "environment")
	if err := parseArguments(nil, []string{"context"}); err != nil {
		t.Fatal(err)
	}
	if input("context", "default") != "environment" {
		t.Fatal("environment ignored")
	}
	if err := parseArguments([]string{"--context=path with spaces; $(not executed)"}, []string{"context"}); err != nil {
		t.Fatal(err)
	}
	if input("context", "default") != "path with spaces; $(not executed)" {
		t.Fatal("argument was modified")
	}
	if err := parseArguments([]string{"--context="}, []string{"context"}); err != nil {
		t.Fatal(err)
	}
	if input("context", "default") != "" {
		t.Fatal("explicit empty argument did not override environment")
	}
	for _, args := range [][]string{{"merge-image-index"}, {"--operation=merge-image-index"}, {"--unknown=value"}} {
		if err := parseArguments(args, []string{"context"}); err == nil {
			t.Fatal("accepted runtime dispatch or unknown input")
		}
	}
}
