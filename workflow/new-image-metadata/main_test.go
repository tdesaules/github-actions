package main

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

func fixture(t *testing.T, mode int64, corrupt bool) string {
	t.Helper()
	var data bytes.Buffer
	w := tar.NewWriter(&data)
	if err := w.WriteHeader(&tar.Header{Name: "tmp", Mode: mode, Typeflag: tar.TypeDir}); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	blobs := map[string][]byte{}
	add := func(data []byte) descriptor {
		digest := fmt.Sprintf("sha256:%x", sha256.Sum256(data))
		blobs["blobs/sha256/"+digest[7:]] = data
		return descriptor{Digest: digest, Size: int64(len(data))}
	}
	l := add(data.Bytes())
	c := add([]byte(`{"architecture":"amd64","os":"linux"}`))
	manifest, _ := json.Marshal(map[string]any{"config": c, "layers": []descriptor{l}})
	m := add(manifest)
	blobs["index.json"], _ = json.Marshal(map[string]any{"manifests": []descriptor{m}})
	if corrupt {
		blobs["blobs/sha256/"+l.Digest[7:]][0] = 'x'
	}
	path := filepath.Join(t.TempDir(), "archive with spaces.tar")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	w = tar.NewWriter(f)
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
	return path
}

func TestArchivePlatformPermissionsAndCorruption(t *testing.T) {
	good := fixture(t, 01777, false)
	if _, err := inspect(good, "amd64"); err != nil {
		t.Fatal(err)
	}
	if _, err := inspect(good, "arm64"); err == nil {
		t.Fatal("accepted wrong architecture")
	}
	for _, path := range []string{fixture(t, 0755, false), fixture(t, 01777, true)} {
		if _, err := inspect(path, "amd64"); err == nil {
			t.Fatal("accepted invalid archive")
		}
	}
}
