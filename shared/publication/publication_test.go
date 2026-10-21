package publication

import (
	"archive/tar"
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tdesaules/github-actions/shared/catalog"
)

func fixtureArchive(t *testing.T, path, arch string) {
	t.Helper()
	var layer bytes.Buffer
	tw := tar.NewWriter(&layer)
	if err := tw.WriteHeader(&tar.Header{Name: "tmp", Mode: 01777, Typeflag: tar.TypeDir}); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	blobs := map[string][]byte{}
	add := func(data []byte, media string) map[string]any {
		digest := Hash(data)
		blobs["blobs/sha256/"+strings.TrimPrefix(digest, "sha256:")] = data
		return map[string]any{"digest": digest, "size": len(data), "mediaType": media}
	}
	l := add(layer.Bytes(), "application/vnd.oci.image.layer.v1.tar")
	config, _ := json.Marshal(map[string]any{"architecture": arch, "os": "linux", "rootfs": map[string]any{"type": "layers", "diff_ids": []any{l["digest"]}}})
	c := add(config, "application/vnd.oci.image.config.v1+json")
	manifest, _ := json.Marshal(map[string]any{"schemaVersion": 2, "mediaType": "application/vnd.oci.image.manifest.v1+json", "config": c, "layers": []any{l}})
	m := add(manifest, "application/vnd.oci.image.manifest.v1+json")
	blobs["index.json"], _ = json.Marshal(map[string]any{"schemaVersion": 2, "manifests": []any{m}})
	blobs["oci-layout"] = []byte(`{"imageLayoutVersion":"1.0.0"}`)
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	w := tar.NewWriter(f)
	for name, data := range blobs {
		if err = w.WriteHeader(&tar.Header{Name: name, Mode: 0644, Typeflag: tar.TypeReg, Size: int64(len(data))}); err != nil {
			t.Fatal(err)
		}
		if _, err = w.Write(data); err != nil {
			t.Fatal(err)
		}
	}
	if err = w.Close(); err != nil {
		t.Fatal(err)
	}
}
func gitTest(t *testing.T, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-c", "commit.gpgsign=false", "-c", "tag.gpgsign=false"}, args...)...)
	data, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %s: %v", args, data, err)
	}
	return strings.TrimSpace(string(data))
}
func fixture(t *testing.T) (string, string) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("real Git regression requires git")
	}
	root := t.TempDir()
	t.Chdir(root)
	gitTest(t, "init", "-b", "main")
	gitTest(t, "config", "user.name", "Fixture")
	gitTest(t, "config", "user.email", "fixture@example.invalid")
	gitTest(t, "config", "commit.gpgsign", "false")
	gitTest(t, "config", "tag.gpgsign", "false")
	if err := os.MkdirAll("check/test-fixture", 0755); err != nil {
		t.Fatal(err)
	}
	for path, data := range map[string]string{"check/test-fixture/action.yml": "runs:\n  image: docker://ghcr.io/fixture/github-actions/check-test-fixture:unpublished\n", "check/test-fixture/Containerfile": "FROM scratch AS final\n", ".gitignore": ".build/\n"} {
		if err := os.WriteFile(path, []byte(data), 0644); err != nil {
			t.Fatal(err)
		}
	}
	gitTest(t, "add", ".")
	gitTest(t, "commit", "-m", "feat: initial")
	base := gitTest(t, "rev-parse", "HEAD")
	directory := ".build/publication"
	for _, arch := range []string{"amd64", "arm64"} {
		dir := filepath.Join(directory, arch)
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
		archive := filepath.Join(dir, "image.oci.tar")
		fixtureArchive(t, archive, arch)
		sum, err := Checksum(archive)
		if err != nil {
			t.Fatal(err)
		}
		if err = catalog.Write(filepath.Join(dir, "metadata.json"), map[string]any{"image": "check-test-fixture", "arch": arch, "version": "candidate", "revision": base, "tested": true, "archive_sha256": sum}); err != nil {
			t.Fatal(err)
		}
	}
	if err := catalog.Write(filepath.Join(directory, "release-plan.json"), ReleasePlan{Base: base, Version: "0.1.0", Changed: true}); err != nil {
		t.Fatal(err)
	}
	tools := t.TempDir()
	matrix := filepath.Join(tools, "matrix")
	if err := os.WriteFile(matrix, []byte("#!/bin/sh\nprintf '%s\\n' 'matrix={\"include\":[{\"image\":\"check-test-fixture\",\"arch\":\"amd64\",\"rebuild\":true},{\"image\":\"check-test-fixture\",\"arch\":\"arm64\",\"rebuild\":true}]}'\n"), 0755); err != nil {
		t.Fatal(err)
	}
	old := matrixBinary
	matrixBinary = matrix
	t.Cleanup(func() { matrixBinary = old })
	return directory, base
}
func TestPrepareRejectsUnsafeArtifacts(t *testing.T) {
	for _, scenario := range []string{"valid", "missing", "duplicate", "stale", "checksum", "architecture"} {
		t.Run(scenario, func(t *testing.T) {
			directory, _ := fixture(t)
			metadata := filepath.Join(directory, "amd64", "metadata.json")
			var m map[string]any
			if err := catalog.Read(metadata, &m); err != nil {
				t.Fatal(err)
			}
			switch scenario {
			case "missing":
				if err := os.Remove(metadata); err != nil {
					t.Fatal(err)
				}
			case "duplicate":
				if err := catalog.Write(filepath.Join(directory, "duplicate", "metadata.json"), m); err != nil {
					t.Fatal(err)
				}
			case "stale":
				m["revision"] = "wrong"
				if err := catalog.Write(metadata, m); err != nil {
					t.Fatal(err)
				}
			case "checksum":
				m["archive_sha256"] = "wrong"
				if err := catalog.Write(metadata, m); err != nil {
					t.Fatal(err)
				}
			case "architecture":
				archive := filepath.Join(directory, "amd64", "image.oci.tar")
				fixtureArchive(t, archive, "arm64")
				sum, _ := Checksum(archive)
				m["archive_sha256"] = sum
				if err := catalog.Write(metadata, m); err != nil {
					t.Fatal(err)
				}
			}
			err := Prepare(directory, "fixture", filepath.Join(directory, "release-plan.json"))
			if (err == nil) != (scenario == "valid") {
				t.Fatalf("%s: %v", scenario, err)
			}
		})
	}
}
func TestAtomicReleaseAndRecovery(t *testing.T) {
	for _, scenario := range []string{"success", "advanced-main", "rejected-push"} {
		t.Run(scenario, func(t *testing.T) {
			directory, base := fixture(t)
			if err := Prepare(directory, "fixture", filepath.Join(directory, "release-plan.json")); err != nil {
				t.Fatal(err)
			}
			state, err := Load(directory)
			if err != nil {
				t.Fatal(err)
			}
			for _, image := range state.Images {
				image.Signed = true
				image.Published = true
			}
			if err = Save(directory, state); err != nil {
				t.Fatal(err)
			}
			remote := filepath.Join(t.TempDir(), "remote.git")
			gitTest(t, "init", "--bare", remote)
			gitTest(t, "remote", "add", "origin", remote)
			gitTest(t, "push", "origin", "HEAD:main")
			if scenario == "advanced-main" {
				advance := gitTest(t, "commit-tree", "HEAD^{tree}", "-p", "HEAD", "-m", "concurrent update")
				gitTest(t, "push", "origin", advance+":refs/heads/main")
			}
			if scenario == "rejected-push" {
				if err = os.MkdirAll(filepath.Join(remote, "hooks"), 0755); err != nil {
					t.Fatal(err)
				}
				if err = os.WriteFile(filepath.Join(remote, "hooks/pre-receive"), []byte("#!/bin/sh\nexit 1\n"), 0755); err != nil {
					t.Fatal(err)
				}
			}
			tools := t.TempDir()
			if err = os.WriteFile(filepath.Join(tools, "git-cliff"), []byte("#!/bin/sh\nprintf '# Changelog\\n\\n## 0.1.0\\n'\n"), 0755); err != nil {
				t.Fatal(err)
			}
			t.Setenv("PATH", tools+string(os.PathListSeparator)+os.Getenv("PATH"))
			t.Setenv("INPUT_TOKEN", "local-fixture-token")
			err = Finish(directory)
			if scenario == "success" {
				if err != nil {
					t.Fatal(err)
				}
				main := gitTest(t, "--git-dir", remote, "rev-parse", "refs/heads/main")
				tag := gitTest(t, "--git-dir", remote, "rev-parse", "refs/tags/0.1.0")
				if main != tag || main == base {
					t.Fatal("refs not published atomically")
				}
			} else {
				if err == nil {
					t.Fatal("unsafe release accepted")
				}
				if gitTest(t, "--git-dir", remote, "tag", "--list", "0.1.0") != "" {
					t.Fatal("failed release left remote tag")
				}
				if scenario == "rejected-push" && gitTest(t, "--git-dir", remote, "rev-parse", "refs/heads/main") != base {
					t.Fatal("rejected push changed main")
				}
			}
		})
	}
}
