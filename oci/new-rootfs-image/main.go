package main

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/tdesaules/github-actions/shared/component"
	"github.com/tdesaules/github-actions/shared/pantavisor"
	"github.com/tdesaules/github-actions/shared/workflow"
	"io"
	"os"
	"path/filepath"
	"sort"
)

type descriptor struct {
	MediaType string `json:"mediaType"`
	Digest    string `json:"digest"`
	Size      int64  `json:"size"`
}

func run() error {
	directory, output := pantavisor.Input("directory", "dist/component"), pantavisor.Input("output", "dist/image.oci.tar")
	if err := pantavisor.Parse(); err != nil {
		return err
	}
	metadata, sourceDigest, err := component.Verify(*directory)
	if err != nil {
		return err
	}
	if metadata.Kind != "rootfs" || (metadata.Name != "core" && metadata.Name != "k0s") {
		return fmt.Errorf("expected Core or k0s rootfs")
	}
	if _, err := os.Lstat(*output); !os.IsNotExist(err) {
		return fmt.Errorf("output already exists")
	}
	temp, err := os.MkdirTemp("", "rootfs-oci-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(temp)
	if err := os.MkdirAll(filepath.Join(temp, "blobs/sha256"), 0755); err != nil {
		return err
	}
	put := func(value any, media string) (descriptor, error) {
		data, e := json.Marshal(value)
		if e != nil {
			return descriptor{}, e
		}
		hash := sha256.Sum256(data)
		sum := hex.EncodeToString(hash[:])
		e = os.WriteFile(filepath.Join(temp, "blobs/sha256", sum), data, 0644)
		return descriptor{media, "sha256:" + sum, int64(len(data))}, e
	}
	compressed := metadata.Files["rootfs.tar.gz"]
	input, err := os.Open(filepath.Join(*directory, "rootfs.tar.gz"))
	if err != nil {
		return err
	}
	defer input.Close()
	reader, err := gzip.NewReader(input)
	if err != nil {
		return err
	}
	hash := sha256.New()
	if _, err := io.Copy(hash, reader); err != nil {
		return err
	}
	if err := reader.Close(); err != nil {
		return err
	}
	diffID := "sha256:" + hex.EncodeToString(hash.Sum(nil))
	if _, err := input.Seek(0, 0); err != nil {
		return err
	}
	layer, err := os.Create(filepath.Join(temp, "blobs/sha256", compressed.SHA256))
	if err != nil {
		return err
	}
	_, err = io.Copy(layer, input)
	closeErr := layer.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	var contract struct {
		Entrypoint []string `json:"entrypoint"`
	}
	if err := pantavisor.ReadJSON(filepath.Join(*directory, "runtime.json"), &contract); err != nil {
		return err
	}
	if len(contract.Entrypoint) == 0 || !filepath.IsAbs(contract.Entrypoint[0]) {
		return fmt.Errorf("runtime contract requires an absolute entrypoint")
	}
	configuration := map[string]any{"architecture": "arm64", "os": "linux", "config": map[string]any{"User": "0:0", "Entrypoint": contract.Entrypoint, "Labels": map[string]string{"org.opencontainers.image.source": "https://github.com/" + metadata.Source["repository"].(string), "org.opencontainers.image.version": metadata.Version, "org.opencontainers.image.revision": metadata.Source["commit"].(string), "org.opencontainers.image.title": "pantavisor-" + metadata.Name}}, "rootfs": map[string]any{"type": "layers", "diff_ids": []string{diffID}}}
	config, err := put(configuration, "application/vnd.oci.image.config.v1+json")
	if err != nil {
		return err
	}
	manifest, err := put(map[string]any{"schemaVersion": 2, "mediaType": "application/vnd.oci.image.manifest.v1+json", "config": config, "layers": []descriptor{{"application/vnd.oci.image.layer.v1.tar+gzip", "sha256:" + compressed.SHA256, compressed.Size}}}, "application/vnd.oci.image.manifest.v1+json")
	if err != nil {
		return err
	}
	if err := pantavisor.WriteJSON(filepath.Join(temp, "index.json"), map[string]any{"schemaVersion": 2, "manifests": []descriptor{manifest}}); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(temp, "oci-layout"), []byte(`{"imageLayoutVersion":"1.0.0"}`), 0644); err != nil {
		return err
	}
	_, after, err := component.Verify(*directory)
	if err != nil || after != sourceDigest {
		return fmt.Errorf("component changed while assembling the OCI archive")
	}
	actualLayer, err := component.Hash(filepath.Join(temp, "blobs/sha256", compressed.SHA256))
	if err != nil || actualLayer != compressed.SHA256 {
		return fmt.Errorf("staged OCI layer differs from the component")
	}
	if err := os.MkdirAll(filepath.Dir(*output), 0755); err != nil {
		return err
	}
	archive, err := os.OpenFile(*output, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	success := false
	defer func() {
		archive.Close()
		if !success {
			os.Remove(*output)
		}
	}()
	writer := tar.NewWriter(archive)
	var files []string
	err = filepath.WalkDir(temp, func(path string, entry os.DirEntry, e error) error {
		if e != nil {
			return e
		}
		if !entry.IsDir() {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		return err
	}
	sort.Strings(files)
	for _, path := range files {
		info, err := os.Stat(path)
		if err != nil {
			return err
		}
		name, _ := filepath.Rel(temp, path)
		if err := writer.WriteHeader(&tar.Header{Name: filepath.ToSlash(name), Mode: 0644, Size: info.Size()}); err != nil {
			return err
		}
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		_, err = io.Copy(writer, f)
		f.Close()
		if err != nil {
			return err
		}
	}
	if err := writer.Close(); err != nil {
		return err
	}
	if err := archive.Close(); err != nil {
		return err
	}
	success = true
	if err := workflow.Output("image-digest", manifest.Digest); err != nil {
		return err
	}
	return workflow.Output("archive", *output)
}
func main() { pantavisor.Main("new-rootfs-image OCI 1", run) }
