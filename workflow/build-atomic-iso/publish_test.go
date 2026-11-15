package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestPublicationPreservesExactBytesAndSidecars(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := os.WriteFile("source.iso", []byte("ISO fixture"), 0644); err != nil {
		t.Fatal(err)
	}
	sum, err := hash("source.iso")
	if err != nil {
		t.Fatal(err)
	}
	receipt := []byte("{\"tested\":false,\"run_id\":\"42\"}\n")
	if err := publishISO(context.Background(), "source.iso", "out/media.iso", sum, receipt); err != nil {
		t.Fatal(err)
	}
	got, err := hash("out/media.iso")
	if err != nil || got != sum {
		t.Fatal("copied bytes changed")
	}
	check, err := os.ReadFile("out/media.iso-CHECKSUM")
	if err != nil || string(check) != sum+"  media.iso\n" {
		t.Fatal("incorrect sidecar")
	}
	r, err := os.ReadFile("out/media.iso.build.json")
	if err != nil || string(r) != string(receipt) {
		t.Fatal("incorrect receipt")
	}
}
func TestPublicationCannotOverwriteOrEscape(t *testing.T) {
	for _, collision := range []string{"out/media.iso", "out/media.iso-CHECKSUM", "out/media.iso.build.json"} {
		t.Run(collision, func(t *testing.T) {
			t.Chdir(t.TempDir())
			_ = os.Mkdir("out", 0755)
			if err := os.WriteFile("source.iso", []byte("source"), 0644); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(collision, []byte("other writer"), 0644); err != nil {
				t.Fatal(err)
			}
			sum, _ := hash("source.iso")
			if publishISO(context.Background(), "source.iso", "out/media.iso", sum, nil) == nil {
				t.Fatal("collision accepted")
			}
			for _, name := range []string{"out/media.iso", "out/media.iso-CHECKSUM", "out/media.iso.build.json"} {
				data, err := os.ReadFile(name)
				if name == collision {
					if err != nil || string(data) != "other writer" {
						t.Fatal("overwrote unrelated file")
					}
				} else if !os.IsNotExist(err) {
					t.Fatal("failed publication left partial output")
				}
			}
		})
	}
	t.Chdir(t.TempDir())
	outside := t.TempDir()
	if err := os.Symlink(outside, "escape"); err != nil {
		t.Fatal(err)
	}
	if publishISO(context.Background(), "absent", "escape/media.iso", "", nil) == nil {
		t.Fatal("escaped root")
	}
	entries, err := os.ReadDir(outside)
	if err != nil || len(entries) > 0 {
		t.Fatal("modified outside workspace")
	}
}
func TestPublicationRollbackOnCancellationAndHashMismatch(t *testing.T) {
	for _, cancelled := range []bool{false, true} {
		t.Run(map[bool]string{false: "hash", true: "cancel"}[cancelled], func(t *testing.T) {
			t.Chdir(t.TempDir())
			if err := os.WriteFile("source.iso", []byte("fixture"), 0644); err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if cancelled {
				cancel()
			}
			if publishISO(ctx, "source.iso", "out/media.iso", "invalid", nil) == nil {
				t.Fatal("invalid publication succeeded")
			}
			for _, name := range []string{"media.iso", "media.iso-CHECKSUM", "media.iso.build.json"} {
				if _, err := os.Lstat(filepath.Join("out", name)); !os.IsNotExist(err) {
					t.Fatal("partial output survived")
				}
			}
		})
	}
}
