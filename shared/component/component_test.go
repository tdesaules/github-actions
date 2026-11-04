package component

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func fixture(t *testing.T, kind string) (string, *Descriptor) {
	t.Helper()
	root := filepath.Join(t.TempDir(), "component with spaces")
	if err := os.Mkdir(root, 0755); err != nil {
		t.Fatal(err)
	}
	metadata := &Descriptor{Schema: 2, Kind: kind, Name: "core", Version: "0.1.0", Architecture: "arm64",
		Provides: []string{}, Requires: []string{"host-network"}, Features: []string{},
		Source: map[string]any{"repository": "fixture/core", "commit": strings.Repeat("a", 40), "treeSha256": strings.Repeat("b", 64), "dirty": true},
		Files:  map[string]File{}}
	write(t, filepath.Join(root, "build-provenance.json"), map[string]any{"schema": 1, "source": metadata.Source})
	names := []string{"sbom.spdx.tar.zst"}
	if kind == "rootfs" {
		metadata.RuntimeABI = "pantavisor-core-v1"
		// k0s currently omits the optional runtime provides field.
		write(t, filepath.Join(root, "runtime.json"), map[string]any{"schema": 1, "id": metadata.RuntimeABI, "architecture": "arm64", "requires": metadata.Requires})
		names = append(names, "rootfs.squashfs", "rootfs.tar.gz", "lxc.conf.in", "packages.manifest")
	} else {
		metadata.Name = "bsp"
		metadata.BootContracts = []string{"emmc-file-store-v1"}
		write(t, filepath.Join(root, "bsp.json"), map[string]any{"bootContracts": metadata.BootContracts})
		names = append(names, "kernel.img", "initramfs-base.cpio.gz", "vendor-root.tar.gz", "selection.json")
	}
	for _, name := range names {
		if err := os.WriteFile(filepath.Join(root, name), []byte("fixture\n"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	refresh(t, root, metadata)
	return root, metadata
}

func write(t *testing.T, filename string, value any) {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filename, data, 0644); err != nil {
		t.Fatal(err)
	}
}

func refresh(t *testing.T, root string, metadata *Descriptor) {
	t.Helper()
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	metadata.Files = map[string]File{}
	for _, entry := range entries {
		if entry.Name() == "component.json" {
			continue
		}
		filename := filepath.Join(root, entry.Name())
		info, err := os.Stat(filename)
		if err != nil {
			t.Fatal(err)
		}
		digest, err := Hash(filename)
		if err != nil {
			t.Fatal(err)
		}
		metadata.Files[entry.Name()] = File{info.Size(), digest}
	}
	if metadata.Kind == "rootfs" {
		metadata.RootfsSHA256 = metadata.Files["rootfs.squashfs"].SHA256
	}
	write(t, filepath.Join(root, "component.json"), metadata)
}

func TestPortableComponents(t *testing.T) {
	for _, kind := range []string{"rootfs", "bsp"} {
		root, expected := fixture(t, kind)
		got, digest, err := Verify(root)
		if err != nil || got.Name != expected.Name || !strings.HasPrefix(digest, "sha256:") {
			t.Fatalf("%s: %v, %s, %v", kind, got, digest, err)
		}
	}
}

func TestRejectCorruptAndPrivateComponents(t *testing.T) {
	for _, mutation := range []string{"payload", "extra", "missing", "symlink", "descriptor-symlink", "source", "runtime", "private", "escape"} {
		t.Run(mutation, func(t *testing.T) {
			root, metadata := fixture(t, "rootfs")
			switch mutation {
			case "payload":
				write(t, filepath.Join(root, "rootfs.squashfs"), "tampered")
			case "extra":
				write(t, filepath.Join(root, "unexpected"), "extra")
			case "missing":
				if err := os.Remove(filepath.Join(root, "runtime.json")); err != nil {
					t.Fatal(err)
				}
			case "symlink", "descriptor-symlink":
				name := "packages.manifest"
				if mutation == "descriptor-symlink" {
					name = "component.json"
				}
				original := filepath.Join(root, name)
				external := filepath.Join(t.TempDir(), name)
				if err := os.Rename(original, external); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(external, original); err != nil {
					t.Fatal(err)
				}
			case "source":
				metadata.Source["dirty"] = false
				write(t, filepath.Join(root, "component.json"), metadata)
			case "runtime":
				metadata.RuntimeABI = "incompatible-v2"
				write(t, filepath.Join(root, "component.json"), metadata)
			case "private":
				metadata.Kind = "device-state"
				write(t, filepath.Join(root, "component.json"), metadata)
			case "escape":
				metadata.Files["../outside"] = metadata.Files["packages.manifest"]
				delete(metadata.Files, "packages.manifest")
				write(t, filepath.Join(root, "component.json"), metadata)
			}
			if _, _, err := Verify(root); err == nil {
				t.Fatal("invalid component accepted")
			}
		})
	}
}

func TestMissingInventoryFields(t *testing.T) {
	var value File
	for _, data := range []string{`{}`, `{"size":0}`, `{"sha256":"abc"}`, `{"size":null,"sha256":"abc"}`, `{"size":0.5,"sha256":"abc"}`} {
		if err := json.Unmarshal([]byte(data), &value); err == nil {
			t.Fatalf("accepted %s", data)
		}
	}
}
