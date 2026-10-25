package candidate

import (
	"archive/tar"
	"os"
	"path/filepath"
	"testing"
)

func layerFile(t *testing.T, entries []*tar.Header) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "layer.tar")
	f, err := os.Create(p)
	if err != nil {
		t.Fatal(err)
	}
	w := tar.NewWriter(f)
	for _, h := range entries {
		if err = w.WriteHeader(h); err != nil {
			t.Fatal(err)
		}
	}
	if err = w.Close(); err != nil {
		t.Fatal(err)
	}
	if err = f.Close(); err != nil {
		t.Fatal(err)
	}
	return p
}
func TestLayerPathsAndPermissions(t *testing.T) {
	for _, entries := range [][]*tar.Header{
		{{Name: "../escape", Typeflag: tar.TypeReg, Mode: 0644}},
		{{Name: "alias", Typeflag: tar.TypeSymlink, Linkname: "../../escape"}},
		{{Name: "alias", Typeflag: tar.TypeLink, Linkname: "../escape"}},
		{{Name: "device", Typeflag: tar.TypeChar, Mode: 0666}},
	} {
		if err := extract(t.TempDir(), layerFile(t, entries), "application/vnd.oci.image.layer.v1.tar"); err == nil {
			t.Fatal("unsafe layer accepted")
		}
	}
	root := t.TempDir()
	first := layerFile(t, []*tar.Header{{Name: "tmp", Typeflag: tar.TypeDir, Mode: 01777}, {Name: "file", Typeflag: tar.TypeReg, Mode: 0644}, {Name: "alias", Typeflag: tar.TypeSymlink, Linkname: "/file"}})
	if err := extract(root, first, "application/vnd.oci.image.layer.v1.tar"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "alias")); err != nil {
		t.Fatal("container-absolute symlink was not confined", err)
	}
	info, err := os.Stat(filepath.Join(root, "tmp"))
	if err != nil || info.Mode()&os.ModeSticky == 0 {
		t.Fatal("sticky bit lost", err)
	}
	second := layerFile(t, []*tar.Header{{Name: ".wh.file", Typeflag: tar.TypeReg, Mode: 0644}})
	if err = extract(root, second, "application/vnd.oci.image.layer.v1.tar"); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(filepath.Join(root, "file")); !os.IsNotExist(err) {
		t.Fatal("whiteout ignored")
	}
}
func TestInstallRefusesOrdinaryProcess(t *testing.T) {
	if _, err := os.Stat("/.candidate-runtime"); err == nil {
		t.Skip("already in an explicitly disposable runtime")
	}
	root := Root{Path: t.TempDir()}
	if root.Install() == nil {
		t.Fatal("candidate install allowed outside disposable image")
	}
}
