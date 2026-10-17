package runtimeaction

import (
	"archive/tar"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type ociDescriptor struct {
	MediaType   string            `json:"mediaType"`
	Digest      string            `json:"digest"`
	Size        int64             `json:"size"`
	Platform    *ociPlatform      `json:"platform,omitempty"`
	Annotations map[string]string `json:"annotations,omitempty"`
}

type ociPlatform struct {
	Architecture string `json:"architecture"`
	OS           string `json:"os"`
}

type ociIndex struct {
	SchemaVersion int             `json:"schemaVersion"`
	MediaType     string          `json:"mediaType,omitempty"`
	Manifests     []ociDescriptor `json:"manifests"`
}

func assembleIndex(sources []string, layout string) ([]byte, error) {
	blobs := filepath.Join(layout, "blobs", "sha256")
	if err := os.MkdirAll(blobs, 0755); err != nil {
		return nil, err
	}
	readBlob := func(d ociDescriptor) ([]byte, error) {
		if !digestPattern.MatchString(d.Digest) {
			return nil, errors.New("invalid blob digest")
		}
		b, err := os.ReadFile(filepath.Join(blobs, strings.TrimPrefix(d.Digest, "sha256:")))
		if err != nil {
			return nil, err
		}
		if int64(len(b)) != d.Size || fmt.Sprintf("sha256:%x", sha256.Sum256(b)) != d.Digest {
			return nil, errors.New("OCI descriptor does not match blob")
		}
		return b, nil
	}
	index := ociIndex{SchemaVersion: 2, MediaType: "application/vnd.oci.image.index.v1+json"}
	seen := map[string]bool{}
	for _, source := range sources {
		f, err := os.Open(source)
		if err != nil {
			return nil, err
		}
		var local ociIndex
		err = func() error {
			defer f.Close()
			reader := tar.NewReader(f)
			for {
				header, err := reader.Next()
				if errors.Is(err, io.EOF) {
					break
				}
				if err != nil {
					return err
				}
				if header.Name == "index.json" {
					if err = json.NewDecoder(io.LimitReader(reader, 1<<20)).Decode(&local); err != nil {
						return err
					}
				} else if strings.HasPrefix(header.Name, "blobs/sha256/") && header.Typeflag == tar.TypeReg {
					digest := strings.TrimPrefix(header.Name, "blobs/sha256/")
					if !digestPattern.MatchString("sha256:" + digest) {
						return errors.New("invalid archive blob path")
					}
					out, err := os.Create(filepath.Join(blobs, digest))
					if err != nil {
						return err
					}
					hash := sha256.New()
					_, err = io.Copy(io.MultiWriter(out, hash), reader)
					closeErr := out.Close()
					if err != nil {
						return err
					}
					if closeErr != nil {
						return closeErr
					}
					if hex.EncodeToString(hash.Sum(nil)) != digest {
						return errors.New("archive blob checksum mismatch")
					}
				}
			}
			return nil
		}()
		if err != nil {
			return nil, err
		}
		if len(local.Manifests) != 1 {
			return nil, errors.New("each archive must contain exactly one image")
		}
		d := local.Manifests[0]
		manifest, err := readBlob(d)
		if err != nil {
			return nil, err
		}
		var image struct {
			Config ociDescriptor   `json:"config"`
			Layers []ociDescriptor `json:"layers"`
		}
		if err = json.Unmarshal(manifest, &image); err != nil {
			return nil, err
		}
		config, err := readBlob(image.Config)
		if err != nil {
			return nil, err
		}
		var platform ociPlatform
		if err = json.Unmarshal(config, &platform); err != nil {
			return nil, err
		}
		if platform.OS != "linux" || (platform.Architecture != "amd64" && platform.Architecture != "arm64") || seen[platform.Architecture] {
			return nil, errors.New("index requires distinct linux/amd64 and linux/arm64 images")
		}
		for _, layer := range image.Layers {
			if !digestPattern.MatchString(layer.Digest) {
				return nil, errors.New("invalid layer digest")
			}
			info, err := os.Stat(filepath.Join(blobs, strings.TrimPrefix(layer.Digest, "sha256:")))
			if err != nil || info.Size() != layer.Size {
				return nil, errors.New("missing or truncated layer")
			}
		}
		seen[platform.Architecture] = true
		d.Platform, d.Annotations = &platform, nil
		index.Manifests = append(index.Manifests, d)
	}
	if len(seen) != 2 {
		return nil, errors.New("two architectures required")
	}
	sort.Slice(index.Manifests, func(i, j int) bool {
		return index.Manifests[i].Platform.Architecture < index.Manifests[j].Platform.Architecture
	})
	data, err := json.Marshal(index)
	if err != nil {
		return nil, err
	}
	digest := fmt.Sprintf("%x", sha256.Sum256(data))
	if err = os.WriteFile(filepath.Join(blobs, digest), data, 0644); err != nil {
		return nil, err
	}
	root, err := json.Marshal(ociIndex{SchemaVersion: 2, Manifests: []ociDescriptor{{MediaType: index.MediaType, Digest: "sha256:" + digest, Size: int64(len(data)), Annotations: map[string]string{"org.opencontainers.image.ref.name": "index"}}}})
	if err != nil {
		return nil, err
	}
	if err = os.WriteFile(filepath.Join(layout, "index.json"), root, 0644); err != nil {
		return nil, err
	}
	if err = os.WriteFile(filepath.Join(layout, "oci-layout"), []byte(`{"imageLayoutVersion":"1.0.0"}`), 0644); err != nil {
		return nil, err
	}
	return data, nil
}
