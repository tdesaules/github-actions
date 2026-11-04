package pantavisor

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReceiptIntegrityAndSystemProfiles(t *testing.T) {
	root := t.TempDir()
	directory := filepath.Join(root, "dist")
	if err := os.Mkdir(directory, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "payload"), []byte("bound output"), 0644); err != nil {
		t.Fatal(err)
	}
	files, err := Inventory(directory)
	if err != nil {
		t.Fatal(err)
	}
	receipt := Receipt{Schema: 1, Project: "system", Revision: strings.Repeat("a", 40), Version: "0.1.0", Files: files}
	name := filepath.Join(root, "receipt.json")
	if err := WriteJSON(name, receipt); err != nil {
		t.Fatal(err)
	}
	if _, err := CheckReceipt(directory, name, receipt.Revision, false); err != nil {
		t.Fatal(err)
	}
	if _, err := CheckReceipt(directory, name, receipt.Revision, true); err == nil {
		t.Fatal("untested outputs accepted")
	}
	receipt.Tested = true
	receipt.Profiles = []string{"base"}
	WriteJSON(name, receipt)
	if _, err := CheckReceipt(directory, name, receipt.Revision, true); err == nil {
		t.Fatal("partial System qualification accepted")
	}
	receipt.Profiles = []string{"base", "k0s"}
	WriteJSON(name, receipt)
	if _, err := CheckReceipt(directory, name, receipt.Revision, true); err != nil {
		t.Fatal(err)
	}
	if _, err := CheckReceipt(directory, name, strings.Repeat("b", 40), true); err == nil {
		t.Fatal("wrong source accepted")
	}
	os.WriteFile(filepath.Join(directory, "payload"), []byte("corruption"), 0644)
	if _, err := CheckReceipt(directory, name, receipt.Revision, true); err == nil {
		t.Fatal("corrupt output accepted")
	}
}
func TestSourceInventoryRejectsSymlinks(t *testing.T) {
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "original"), []byte("content"), 0644)
	os.Symlink("original", filepath.Join(root, "link"))
	if _, err := Inventory(root); err == nil {
		t.Fatal("symlink accepted")
	}
}
