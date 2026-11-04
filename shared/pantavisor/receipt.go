package pantavisor

import (
	"archive/tar"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"

	"github.com/tdesaules/github-actions/shared/component"
	"github.com/tdesaules/github-actions/shared/workflow"
)

type Receipt struct {
	Schema   int                       `json:"schema"`
	Project  string                    `json:"project"`
	Revision string                    `json:"revision"`
	Version  string                    `json:"version"`
	Tested   bool                      `json:"tested"`
	Profiles []string                  `json:"profiles,omitempty"`
	Files    map[string]component.File `json:"files"`
}

func WriteJSON(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0644)
}
func ReadJSON(path string, value any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, value)
}
func Inventory(directory string) (map[string]component.File, error) {
	if stat, err := os.Lstat(directory); err != nil || !stat.IsDir() {
		return nil, fmt.Errorf("artifact directory must exist and not be a symlink")
	}
	result := map[string]component.File{}
	err := filepath.WalkDir(directory, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		if !entry.Type().IsRegular() {
			return fmt.Errorf("non-regular artifact %s", path)
		}
		relative, err := filepath.Rel(directory, path)
		if err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		sum, err := component.Hash(path)
		if err != nil {
			return err
		}
		result[filepath.ToSlash(relative)] = component.File{Size: info.Size(), SHA256: sum}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if len(result) == 0 {
		return nil, fmt.Errorf("empty artifacts")
	}
	return result, nil
}
func CheckReceipt(directory, path, revision string, tested bool) (Receipt, error) {
	var receipt Receipt
	if err := ReadJSON(path, &receipt); err != nil {
		return receipt, err
	}
	if receipt.Schema != 1 || receipt.Revision != revision || !regexp.MustCompile(`^[a-f0-9]{40}$`).MatchString(revision) || !regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$`).MatchString(receipt.Version) || (tested && !receipt.Tested) {
		return receipt, fmt.Errorf("invalid artifact receipt/source/qualification")
	}
	if receipt.Project != "core" && receipt.Project != "k0s" && receipt.Project != "system" {
		return receipt, fmt.Errorf("invalid artifact project")
	}
	if tested && receipt.Project == "system" && !reflect.DeepEqual(receipt.Profiles, []string{"base", "k0s"}) {
		return receipt, fmt.Errorf("System requires both qualified profiles")
	}
	files, err := Inventory(directory)
	if err != nil {
		return receipt, err
	}
	if !reflect.DeepEqual(receipt.Files, files) {
		return receipt, fmt.Errorf("artifact inventory differs from receipt")
	}
	return receipt, nil
}
func NewReceipt() error {
	directory, project, revision, output := Input("directory", "dist"), Input("project", ""), Input("revision", ""), Input("output", "artifacts/build-receipt.json")
	if err := Parse(); err != nil {
		return err
	}
	if *project != "core" && *project != "k0s" && *project != "system" {
		return fmt.Errorf("invalid project")
	}
	if !regexp.MustCompile(`^[a-f0-9]{40}$`).MatchString(*revision) {
		return fmt.Errorf("invalid revision")
	}
	version, err := os.ReadFile("VERSION")
	if err != nil {
		return err
	}
	files, err := Inventory(*directory)
	if err != nil {
		return err
	}
	own := "component"
	if *project == "system" {
		own = "bsp"
	}
	metadata, _, err := component.Verify(filepath.Join(*directory, own))
	if err != nil {
		return err
	}
	if metadata.Source["commit"] != *revision || metadata.Version != strings.TrimSpace(string(version)) {
		return fmt.Errorf("component source/version differs from build receipt")
	}
	if metadata.Source["dirty"] != false {
		return fmt.Errorf("CI receipts require clean build provenance")
	}
	return WriteJSON(*output, Receipt{Schema: 1, Project: *project, Revision: *revision, Version: strings.TrimSpace(string(version)), Tested: false, Files: files})
}
func TestArtifact() error {
	directory, receipt, revision, tested, expected := Input("directory", "dist"), Input("receipt", "artifacts/build-receipt.json"), Input("revision", ""), Input("require-tested", "false"), Input("receipt-digest", "")
	if err := Parse(); err != nil {
		return err
	}
	if *tested != "true" && *tested != "false" {
		return fmt.Errorf("require-tested must be boolean")
	}
	if *expected != "" {
		sum, err := component.Hash(*receipt)
		if err != nil || sum != *expected {
			return fmt.Errorf("receipt differs from the tag's qualification digest")
		}
	}
	value, err := CheckReceipt(*directory, *receipt, *revision, *tested == "true")
	if err != nil {
		return err
	}
	version, err := os.ReadFile("VERSION")
	if err != nil || strings.TrimSpace(string(version)) != value.Version {
		return fmt.Errorf("receipt VERSION differs from the checked-out project")
	}
	if err := workflow.Output("version", value.Version); err != nil {
		return err
	}
	if _, ok := value.Files["image.oci.tar"]; ok {
		digest, err := ImageDigest(filepath.Join(*directory, "image.oci.tar"))
		if err != nil {
			return err
		}
		if err := workflow.Output("image-digest", digest); err != nil {
			return err
		}
	}
	return workflow.Output("verified", true)
}
func QualifyArtifact() error {
	directory, receipt, revision, output, profile := Input("directory", "dist"), Input("receipt", "artifacts/build-receipt.json"), Input("revision", ""), Input("output", "artifacts/qualified-receipt.json"), Input("profile", "")
	if err := Parse(); err != nil {
		return err
	}
	value, err := CheckReceipt(*directory, *receipt, *revision, false)
	if err != nil {
		return err
	}
	if value.Project == "system" {
		if *profile != "base" && *profile != "k0s" {
			return fmt.Errorf("System qualification requires a profile")
		}
		value.Profiles = []string{*profile}
	} else if *profile != "" {
		return fmt.Errorf("producer qualification has no platform profile")
	}
	value.Tested = true
	return WriteJSON(*output, value)
}
func MergeQualification() error {
	base, cluster, revision, output := Input("base", "artifacts/qualified-receipt.json"), Input("k0s", "artifacts/profile-k0s/qualified-receipt.json"), Input("revision", ""), Input("output", "artifacts/qualified-receipt.json")
	if err := Parse(); err != nil {
		return err
	}
	a, err := CheckReceipt("dist", *base, *revision, false)
	if err != nil {
		return err
	}
	b, err := CheckReceipt("dist", *cluster, *revision, false)
	if err != nil {
		return err
	}
	if a.Project != "system" || b.Project != "system" || !a.Tested || !b.Tested || a.Version != b.Version || !reflect.DeepEqual(a.Profiles, []string{"base"}) || !reflect.DeepEqual(b.Profiles, []string{"k0s"}) || !reflect.DeepEqual(a.Files, b.Files) {
		return fmt.Errorf("both System profiles must qualify the same artifacts")
	}
	a.Profiles = []string{"base", "k0s"}
	return WriteJSON(*output, a)
}
func ImageDigest(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	reader := tar.NewReader(file)
	for {
		header, err := reader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", err
		}
		if strings.TrimPrefix(header.Name, "./") != "index.json" {
			continue
		}
		var index struct {
			Manifests []struct {
				Digest string `json:"digest"`
			} `json:"manifests"`
		}
		if err := json.NewDecoder(io.LimitReader(reader, 1024*1024)).Decode(&index); err != nil {
			return "", err
		}
		if len(index.Manifests) != 1 || !regexp.MustCompile(`^sha256:[a-f0-9]{64}$`).MatchString(index.Manifests[0].Digest) {
			return "", fmt.Errorf("expected a single OCI archive root manifest")
		}
		return index.Manifests[0].Digest, nil
	}
	return "", fmt.Errorf("OCI archive index missing")
}
