package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"

	"github.com/tdesaules/github-actions/shared/workflow"
)

type receipt struct {
	SchemaVersion   int    `json:"schema_version"`
	Component       string `json:"component"`
	Arch            string `json:"arch"`
	SourceSHA       string `json:"source_sha"`
	Version         string `json:"version"`
	Archive         string `json:"archive"`
	SHA256          string `json:"sha256"`
	Size            int64  `json:"size"`
	OSArchiveSHA256 string `json:"os_archive_sha256"`
	Tested          bool   `json:"tested"`
	RunID           string `json:"run_id"`
	RunAttempt      string `json:"run_attempt"`
}

var hashPattern = regexp.MustCompile(`^[a-f0-9]{64}$`)

func validate(r receipt, revision, version, osArchive string) error {
	if r.SchemaVersion != 1 || r.Component != "iso" || r.Arch != "amd64" || r.Tested ||
		r.SourceSHA != revision || r.Version != version || r.OSArchiveSHA256 != osArchive ||
		!hashPattern.MatchString(r.SHA256) || !hashPattern.MatchString(osArchive) ||
		!regexp.MustCompile(`^[a-f0-9]{40}$`).MatchString(revision) || version == "" || r.Size <= 0 {
		return fmt.Errorf("ISO build receipt does not match the qualified source OS")
	}
	return nil
}

func hash(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func command(name string, args ...string) ([]byte, error) {
	output, err := exec.Command(name, args...).CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("%s: %w: %s", name, err, output)
	}
	return output, nil
}

func run() error {
	iso := flag.String("iso", "", "ISO to qualify")
	buildReceipt := flag.String("receipt", "", "untested ISO build receipt")
	revision := flag.String("revision", "", "expected source commit")
	version := flag.String("version", "", "expected candidate version")
	osArchive := flag.String("os-archive-sha256", "", "qualified AMD64 OS archive checksum")
	manifest := flag.String("os-manifest-digest", "", "qualified native OS manifest digest")
	imageName := flag.String("image-name", "niri-tomic-os", "embedded OCI directory")
	output := flag.String("output", ".build/iso-qualification.json", "qualification report")
	flag.Parse()
	if flag.NArg() != 0 || *iso == "" || *buildReceipt == "" ||
		!regexp.MustCompile(`^[a-z0-9][a-z0-9._-]*$`).MatchString(*imageName) ||
		!regexp.MustCompile(`^sha256:[a-f0-9]{64}$`).MatchString(*manifest) {
		return fmt.Errorf("ISO, receipt and qualified native manifest required")
	}
	data, err := os.ReadFile(*buildReceipt)
	if err != nil {
		return err
	}
	var r receipt
	if err := json.Unmarshal(data, &r); err != nil {
		return err
	}
	if err := validate(r, *revision, *version, *osArchive); err != nil {
		return err
	}
	isoPath, err := filepath.Abs(*iso)
	if err != nil {
		return err
	}
	receiptPath, err := filepath.Abs(*buildReceipt)
	if err != nil {
		return err
	}
	if receiptPath != isoPath+".build.json" {
		return fmt.Errorf("original build receipt must be stored next to the ISO as ISO.build.json")
	}
	buildRunID, buildRunAttempt := r.RunID, r.RunAttempt
	receiptHash := fmt.Sprintf("%x", sha256.Sum256(data))
	info, err := os.Lstat(*iso)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Size() != r.Size || filepath.Base(*iso) != r.Archive {
		return fmt.Errorf("ISO path, type or size does not match receipt")
	}
	actual, err := hash(*iso)
	if err != nil {
		return err
	}
	if actual != r.SHA256 {
		return fmt.Errorf("ISO checksum mismatch")
	}
	if _, err := command("checkisomd5", "--verbose", *iso); err != nil {
		return err
	}
	boot, err := command("xorriso", "-indev", *iso, "-report_el_torito", "plain")
	if err != nil {
		return err
	}
	for _, platform := range []string{"BIOS", "UEFI"} {
		if !regexp.MustCompile(`El Torito boot img\s*:\s*\d+\s+` + platform + `\s+y\s`).Match(boot) {
			return fmt.Errorf("missing bootable %s catalog entry", platform)
		}
	}
	tmp, err := os.MkdirTemp("", "atomic-iso-test-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	embedded := filepath.Join(tmp, "embedded")
	if _, err := command("xorriso", "-osirrox", "on", "-indev", *iso,
		"-extract", "/"+*imageName, embedded); err != nil {
		return err
	}
	// stdout alone is the manifest; Skopeo diagnostics must not enter its hash.
	inspect := exec.Command("skopeo", "inspect", "--raw", "oci:"+embedded+":45")
	inspect.Stderr = os.Stderr
	raw, err := inspect.Output()
	if err != nil {
		return err
	}
	if fmt.Sprintf("sha256:%x", sha256.Sum256(raw)) != *manifest {
		return fmt.Errorf("ISO embeds a different OS manifest from the qualified archive")
	}
	// Copy verifies every referenced layer/config blob, not just index metadata.
	if _, err := command("skopeo", "copy", "--preserve-digests",
		"oci:"+embedded+":45", "dir:"+filepath.Join(tmp, "verified")); err != nil {
		return err
	}
	checksumPath := *iso + "-CHECKSUM"
	checksumData, err := os.ReadFile(checksumPath)
	if err != nil {
		return err
	}
	if string(checksumData) != actual+"  "+filepath.Base(*iso)+"\n" {
		return fmt.Errorf("ISO checksum sidecar does not match payload")
	}
	checksumHash, err := hash(checksumPath)
	if err != nil {
		return err
	}
	after, err := hash(*iso)
	if err != nil || after != actual {
		return fmt.Errorf("ISO changed during qualification: %v", err)
	}
	receiptAfter, err := hash(*buildReceipt)
	if err != nil || receiptAfter != receiptHash {
		return fmt.Errorf("ISO build receipt changed during qualification: %v", err)
	}
	r.Tested = true
	r.RunID, r.RunAttempt = os.Getenv("GITHUB_RUN_ID"), os.Getenv("GITHUB_RUN_ATTEMPT")
	files := []map[string]string{
		{"path": filepath.Base(*iso), "sha256": actual, "media_type": "application/x-iso9660-image"},
		{"path": filepath.Base(checksumPath), "sha256": checksumHash, "media_type": "text/plain"},
		{"path": filepath.Base(*buildReceipt), "sha256": receiptHash, "media_type": "application/json"},
	}
	report := struct {
		receipt
		Qualification   string              `json:"qualification"`
		BootTested      bool                `json:"boot_tested"`
		Manifest        string              `json:"manifest_digest"`
		BuildRunID      string              `json:"build_run_id"`
		BuildRunAttempt string              `json:"build_run_attempt"`
		Files           []map[string]string `json:"files"`
	}{r, "installer-media", false, *manifest, buildRunID, buildRunAttempt, files}
	data, err = json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	outputPath, err := filepath.Abs(*output)
	if err != nil {
		return err
	}
	for _, input := range []string{*iso, *buildReceipt, checksumPath} {
		inputPath, err := filepath.Abs(input)
		if err != nil {
			return err
		}
		if outputPath == inputPath {
			return fmt.Errorf("qualification output must not overwrite input")
		}
	}
	if err := os.MkdirAll(filepath.Dir(*output), 0755); err != nil {
		return err
	}
	reportFile, err := os.OpenFile(*output, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	_, writeErr := reportFile.Write(append(data, '\n'))
	closeErr := reportFile.Close()
	if writeErr != nil {
		return writeErr
	}
	if closeErr != nil {
		return closeErr
	}
	for key, value := range map[string]any{
		"receipt": *output, "files": files, "directory": filepath.Dir(*iso),
		"sha256": actual, "size": info.Size(),
	} {
		if err := workflow.Output(key, value); err != nil {
			return err
		}
	}
	return nil
}

func main() { workflow.Main("test-atomic-iso 1", run) }
