// Repository-only SYNTHETIC protocol fixtures. Never production qualification.
package main

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

const warning = "SYNTHETIC TEST FIXTURE: NOT BOOTABLE; NO OS USERSPACE QUALIFICATION.\n"
const isoName = "SYNTHETIC-NOT-BOOTABLE.iso"

var epoch = time.Unix(946684800, 0).UTC()

type options struct {
	Directory, Source, Version, Run, Attempt string
	LargeSBOM                                bool
}
type payload struct {
	Path  string `json:"path"`
	SHA   string `json:"sha256"`
	Media string `json:"media_type"`
}
type descriptor struct {
	Media       string            `json:"mediaType"`
	Digest      string            `json:"digest"`
	Size        int               `json:"size"`
	Annotations map[string]string `json:"annotations,omitempty"`
}

func sum(b []byte) string { return fmt.Sprintf("%x", sha256.Sum256(b)) }
func jsonBytes(v any) []byte {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		panic(err)
	} // Only fixed JSON-compatible fixture structures.
	return append(b, '\n')
}
func write(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		return err
	}
	return os.Chtimes(path, epoch, epoch)
}
func tarBytes(files map[string][]byte) ([]byte, error) {
	var b bytes.Buffer
	w := tar.NewWriter(&b)
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		data := files[name]
		if err := w.WriteHeader(&tar.Header{Name: name, Size: int64(len(data)), Mode: 0644, ModTime: epoch, Format: tar.FormatUSTAR}); err != nil {
			return nil, err
		}
		if _, err := w.Write(data); err != nil {
			return nil, err
		}
	}
	if err := w.Close(); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

// layout builds actual OCI bytes; it deliberately contains no runnable OS.
func layout(arch, source, version string) (map[string][]byte, []byte, string, error) {
	layer, err := tarBytes(map[string][]byte{"hello.txt": []byte(warning + "hello " + arch + "\n")})
	if err != nil {
		return nil, nil, "", err
	}
	config := jsonBytes(map[string]any{
		"created": epoch.Format(time.RFC3339), "architecture": arch, "os": "linux",
		"config": map[string]any{"Labels": map[string]string{
			"org.opencontainers.image.revision": source, "org.opencontainers.image.version": version,
			"org.opencontainers.image.title": "SYNTHETIC NOT AN OS", "io.github.atomic-fixtures.synthetic": "true",
		}},
		"rootfs": map[string]any{"type": "layers", "diff_ids": []string{"sha256:" + sum(layer)}},
	})
	desc := func(media string, data []byte) descriptor {
		return descriptor{Media: media, Digest: "sha256:" + sum(data), Size: len(data)}
	}
	manifest := jsonBytes(map[string]any{
		"schemaVersion": 2, "mediaType": "application/vnd.oci.image.manifest.v1+json",
		"config": desc("application/vnd.oci.image.config.v1+json", config),
		"layers": []descriptor{desc("application/vnd.oci.image.layer.v1.tar", layer)},
	})
	md := desc("application/vnd.oci.image.manifest.v1+json", manifest)
	md.Annotations = map[string]string{"org.opencontainers.image.ref.name": "45"}
	files := map[string][]byte{
		"oci-layout":                    []byte("{\"imageLayoutVersion\":\"1.0.0\"}\n"),
		"index.json":                    jsonBytes(map[string]any{"schemaVersion": 2, "manifests": []descriptor{md}}),
		"blobs/sha256/" + sum(config):   config,
		"blobs/sha256/" + sum(layer):    layer,
		"blobs/sha256/" + sum(manifest): manifest,
	}
	archive, err := tarBytes(files)
	return files, archive, md.Digest, err
}

func validate(o *options, env func(string) string) error {
	if o.Directory == "" || filepath.IsAbs(o.Directory) || filepath.Clean(o.Directory) == "." ||
		strings.ContainsAny(o.Directory, "\r\n\x00\\") {
		return fmt.Errorf("directory must be a new workspace-relative path")
	}
	for _, p := range strings.Split(o.Directory, "/") {
		if p == ".." {
			return fmt.Errorf("directory must not traverse parents")
		}
	}
	if !regexp.MustCompile(`^[a-f0-9]{40}$`).MatchString(o.Source) ||
		!regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$`).MatchString(o.Version) {
		return fmt.Errorf("40-hex source and stable version required")
	}
	o.Run, o.Attempt = env("GITHUB_RUN_ID"), env("GITHUB_RUN_ATTEMPT")
	if o.Run == "" && o.Attempt == "" && env("GITHUB_ACTIONS") == "" {
		// Explicit fixture-only defaults; never substitute these inside Actions.
		o.Run, o.Attempt = "1", "1"
	}
	positive := regexp.MustCompile(`^[1-9][0-9]*$`)
	if !positive.MatchString(o.Run) || !positive.MatchString(o.Attempt) {
		return fmt.Errorf("valid current run ID and attempt required")
	}
	return nil
}

// Reject existing output and symlink ancestors before creating any payload.
func prepareDestination(path string) error {
	path = filepath.Clean(path)
	current := "."
	parts := strings.Split(path, string(filepath.Separator))
	for i, part := range parts {
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if err == nil {
			if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || i == len(parts)-1 {
				return fmt.Errorf("output must be new with directory-only nonsymlink ancestors")
			}
		} else if os.IsNotExist(err) {
			if i != len(parts)-1 {
				if err := os.Mkdir(current, 0755); err != nil {
					return err
				}
			}
		} else {
			return err
		}
	}
	return nil
}

func command(ctx context.Context, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	data, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s failed: %w: %s", name, err, data)
	}
	return nil
}

func media(ctx context.Context, tree, output string) error {
	// isomd5sum's fragment checks need enough data beyond the system area.
	// Tiny sub-megabyte images can implant successfully yet fail verification.
	padding := make([]byte, 2*1024*1024)
	copy(padding, warning)
	if err := write(filepath.Join(tree, "SYNTHETIC-PADDING"), padding); err != nil {
		return err
	}
	for _, name := range []string{"bios.img", "efi.img"} {
		blob := make([]byte, 4096)
		copy(blob, warning)
		if err := write(filepath.Join(tree, "boot", name), blob); err != nil {
			return err
		}
	}
	if err := write(filepath.Join(tree, "SYNTHETIC-NOT-BOOTABLE.txt"), []byte(warning)); err != nil {
		return err
	}
	if err := command(ctx, "xorriso", "-as", "mkisofs", "-quiet", "-R",
		"-V", "SYNTHETIC_NOT_BOOTABLE", "--modification-date=2000010100000000",
		"--set_all_file_dates", "2000010100000000",
		"-b", "boot/bios.img", "-no-emul-boot", "-boot-load-size", "4",
		"-eltorito-alt-boot", "-e", "boot/efi.img", "-no-emul-boot",
		"-o", output, tree); err != nil {
		return err
	}
	if err := command(ctx, "implantisomd5", "--supported-iso", output); err != nil {
		return err
	}
	return command(ctx, "checkisomd5", output)
}

func baseReceipt(o options, component, arch, archive string, data []byte) map[string]any {
	return map[string]any{
		"schema_version": 1, "component": component, "arch": arch, "source_sha": o.Source,
		"version": o.Version, "archive": archive, "sha256": sum(data), "run_id": o.Run,
		"run_attempt": o.Attempt, "tested": false, "boot_tested": false,
		"synthetic": true, "fixture_warning": strings.TrimSpace(warning),
	}
}

func generate(ctx context.Context, o options, makeMedia func(context.Context, string, string) error) (map[string]string, error) {
	if err := prepareDestination(o.Directory); err != nil {
		return nil, err
	}
	stage, err := os.MkdirTemp(filepath.Dir(o.Directory), ".atomic-fixtures-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(stage)
	work, err := os.MkdirTemp("", "atomic-fixtures-private-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(work)
	tree := filepath.Join(work, "tree")
	if err = os.MkdirAll(tree, 0755); err != nil {
		return nil, err
	}
	var amdArchive []byte
	var amdManifest string
	zstdChecksums := map[string]string{}
	for _, arch := range []string{"amd64", "arm64"} {
		files, archive, manifest, err := layout(arch, o.Source, o.Version)
		if err != nil {
			return nil, err
		}
		dir := filepath.Join(stage, "qualified", arch)
		if err = write(filepath.Join(dir, "image.oci.tar"), archive); err != nil {
			return nil, err
		}
		r := baseReceipt(o, "os", arch, "image.oci.tar", archive)
		r["tested"], r["qualification"], r["manifest_digest"] = true, "userspace", manifest
		checks := map[string]string{}
		for _, name := range []string{"Fedora version", "Niri configuration", "desktop and keyring packages", "PAM dynamic dependencies", "enabled greeter", "enabled GPU profile", "GPU helper syntax", "GPU memory ceiling"} {
			checks[name] = "SYNTHETIC protocol field only; check NOT executed"
		}
		r["checks"], r["packages"] = checks, []string{"SYNTHETIC-NOT-INSTALLED|1.0.0|" + arch}
		if err = write(filepath.Join(dir, "qualification.json"), jsonBytes(r)); err != nil {
			return nil, err
		}
		if arch == "amd64" {
			amdArchive, amdManifest = archive, manifest
			for name, data := range files {
				if err = write(filepath.Join(tree, "niri-tomic-os", name), data); err != nil {
					return nil, err
				}
			}
		}
		mixed, err := mixedArchive(arch, o.Source, o.Version)
		if err != nil {
			return nil, err
		}
		name := "arm64.oci.tar"
		if arch == "amd64" {
			name = "fixture.oci.tar"
		}
		if err := write(filepath.Join(stage, "zstd", name), mixed); err != nil {
			return nil, err
		}
		zstdChecksums[name] = sum(mixed)
	}
	if err := write(filepath.Join(stage, "zstd/fixture-checksums.json"), jsonBytes(zstdChecksums)); err != nil {
		return nil, err
	}
	isoPath := filepath.Join(work, isoName)
	if err = makeMedia(ctx, tree, isoPath); err != nil {
		return nil, err
	}
	iso, err := os.ReadFile(isoPath)
	if err != nil {
		return nil, err
	}
	build := baseReceipt(o, "iso", "amd64", isoName, iso)
	build["size"], build["os_archive_sha256"] = len(iso), sum(amdArchive)
	buildBytes := jsonBytes(build)
	sidecar := []byte(sum(iso) + "  " + isoName + "\n")
	isoFiles := []payload{
		{isoName, sum(iso), "application/x-iso9660-image"},
		{isoName + "-CHECKSUM", sum(sidecar), "text/plain"},
		{isoName + ".build.json", sum(buildBytes), "application/json"},
	}
	for _, dir := range []string{"raw", "qualified/amd64"} {
		for name, data := range map[string][]byte{isoName: iso, isoName + "-CHECKSUM": sidecar, isoName + ".build.json": buildBytes} {
			if err = write(filepath.Join(stage, dir, name), data); err != nil {
				return nil, err
			}
		}
	}
	build["tested"], build["qualification"], build["manifest_digest"] = true, "installer-media", amdManifest
	build["build_run_id"], build["build_run_attempt"], build["files"] = o.Run, o.Attempt, isoFiles
	if err = write(filepath.Join(stage, "qualified/amd64/iso-qualification.json"), jsonBytes(build)); err != nil {
		return nil, err
	}
	if err = write(filepath.Join(stage, "raw/image.oci.tar"), amdArchive); err != nil {
		return nil, err
	}
	if err = write(filepath.Join(stage, "raw/os-manifest-digest.txt"), []byte(amdManifest+"\n")); err != nil {
		return nil, err
	}
	transfer := []byte(warning + "deterministic ORAS round-trip payload\n")
	if err = write(filepath.Join(stage, "transfer/payload.txt"), transfer); err != nil {
		return nil, err
	}
	if err = write(filepath.Join(stage, "SYNTHETIC-NOT-QUALIFICATION.txt"), []byte(warning)); err != nil {
		return nil, err
	}
	var largeHash string
	if o.LargeSBOM {
		largeHash, err = writeLargeSBOM(ctx, filepath.Join(stage, "security/large.spdx.json"))
		if err != nil {
			return nil, err
		}
	}
	if err = os.Chmod(stage, 0755); err != nil {
		return nil, err
	}
	if err = os.Rename(stage, o.Directory); err != nil {
		return nil, err
	}
	outputs := map[string]string{
		"iso":               filepath.Join(o.Directory, "raw", isoName),
		"build-receipt":     filepath.Join(o.Directory, "raw", isoName+".build.json"),
		"os-archive-sha256": sum(amdArchive), "os-manifest-digest": amdManifest,
		"qualified-directory": filepath.Join(o.Directory, "qualified"),
		"zstd-directory":      filepath.Join(o.Directory, "zstd"),
		"transfer-directory":  filepath.Join(o.Directory, "transfer"),
	}
	if o.LargeSBOM {
		outputs["large-sbom"] = filepath.Join(o.Directory, "security/large.spdx.json")
		outputs["large-sbom-sha256"] = largeHash
	}
	for key, value := range map[string]any{
		"iso-files":               isoFiles,
		"transfer-files":          []payload{{"payload.txt", sum(transfer), "text/plain"}},
		"tampered-transfer-files": []payload{{"payload.txt", sum([]byte("intentionally wrong checksum")), "text/plain"}},
	} {
		data, _ := json.Marshal(value)
		outputs[key] = string(data)
	}
	return outputs, nil
}

func run(args []string) error {
	if len(args) == 1 && args[0] == "--tool-version" {
		fmt.Println("atomic-fixtures SYNTHETIC 1")
		return nil
	}
	o := options{}
	f := flag.NewFlagSet("atomic-fixtures", flag.ContinueOnError)
	f.StringVar(&o.Directory, "directory", "", "new workspace-relative fixture directory")
	f.StringVar(&o.Source, "source-sha", "", "source revision")
	f.StringVar(&o.Version, "version", "1.0.0", "stable fixture version")
	f.BoolVar(&o.LargeSBOM, "large-sbom", false, "generate an additional streamed synthetic SPDX document above 32 MiB")
	if err := f.Parse(args); err != nil {
		return err
	}
	if f.NArg() != 0 {
		return fmt.Errorf("unexpected arguments")
	}
	if err := validate(&o, os.Getenv); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	outputs, err := generate(ctx, o, media)
	if err != nil {
		return err
	}
	var out io.Writer = os.Stdout
	if path := os.Getenv("GITHUB_OUTPUT"); path != "" {
		f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0600)
		if err != nil {
			return err
		}
		defer f.Close()
		out = f
	}
	keys := make([]string, 0, len(outputs))
	for key := range outputs {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if _, err := fmt.Fprintf(out, "%s=%s\n", key, outputs[key]); err != nil {
			return err
		}
	}
	fmt.Fprintln(os.Stderr, strings.TrimSpace(warning))
	return nil
}
func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
