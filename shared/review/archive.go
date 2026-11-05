package review

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path"
	"regexp"
	"strings"
	"unicode/utf8"
)

// Inventory is reconstructed from checksum-verified OCI layers without
// extracting or executing their contents. Syft does not record --exclude in
// its descriptor, so that descriptor alone cannot prove component absence.
type Inventory struct {
	ManifestDigest string
	Files          map[string]inventoryFile
}

type inventoryFile struct {
	Kind   string `json:"kind"`
	Link   string `json:"link"`
	SHA256 string `json:"sha256"`
	Mode   int64  `json:"mode"`
	UID    int    `json:"uid"`
	GID    int    `json:"gid"`
}

// Fingerprint binds inventoried paths, payloads, links, modes and ownership.
// Timestamps and OCI labels are excluded to permit new run IDs for the same tree.
func (inventory *Inventory) Fingerprint() (string, error) {
	data, err := json.Marshal(struct {
		Format string                   `json:"format"`
		Files  map[string]inventoryFile `json:"files"`
	}{"oci-files-v1", inventory.Files})
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", sha256.Sum256(data)), nil
}

type archiveSection struct{ offset, size int64 }
type archiveDescriptor struct {
	Digest    string `json:"digest"`
	Size      int64  `json:"size"`
	MediaType string `json:"mediaType"`
}

func ReadInventory(filename string, sbom SBOM) (*Inventory, error) {
	f, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return nil, fmt.Errorf("OCI archive must be a regular file")
	}
	entries := map[string]archiveSection{}
	outer := tar.NewReader(f)
	for {
		h, err := outer.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if h.Typeflag == tar.TypeDir {
			continue
		}
		name := strings.TrimPrefix(h.Name, "./")
		if h.Typeflag != tar.TypeReg && h.Typeflag != tar.TypeRegA {
			return nil, fmt.Errorf("non-regular OCI archive entry")
		}
		if path.Clean(name) != name || strings.HasPrefix(name, "/") || strings.Contains("/"+name+"/", "/../") {
			return nil, fmt.Errorf("unsafe OCI archive path")
		}
		if _, exists := entries[name]; exists {
			return nil, fmt.Errorf("duplicate OCI archive path")
		}
		offset, err := f.Seek(0, io.SeekCurrent)
		if err != nil {
			return nil, err
		}
		entries[name] = archiveSection{offset, h.Size}
	}
	if err := zeroPadding(f); err != nil {
		return nil, err
	}
	readJSON := func(section archiveSection, target any) error {
		if section.size <= 0 || section.size > 16<<20 {
			return fmt.Errorf("invalid OCI metadata size")
		}
		data, err := io.ReadAll(io.NewSectionReader(f, section.offset, section.size))
		if err != nil {
			return err
		}
		return json.Unmarshal(data, target)
	}
	verified := map[string]archiveSection{}
	blob := func(d archiveDescriptor) (archiveSection, error) {
		if !regexp.MustCompile(`^sha256:[a-f0-9]{64}$`).MatchString(d.Digest) {
			return archiveSection{}, fmt.Errorf("invalid OCI digest")
		}
		s, ok := entries["blobs/sha256/"+strings.TrimPrefix(d.Digest, "sha256:")]
		if !ok || s.size != d.Size || s.size < 0 {
			return archiveSection{}, fmt.Errorf("missing or inconsistent OCI blob")
		}
		if _, ok := verified[d.Digest]; !ok {
			hash := sha256.New()
			if _, err := io.Copy(hash, io.NewSectionReader(f, s.offset, s.size)); err != nil {
				return archiveSection{}, err
			}
			if fmt.Sprintf("sha256:%x", hash.Sum(nil)) != d.Digest {
				return archiveSection{}, fmt.Errorf("OCI blob checksum mismatch")
			}
			verified[d.Digest] = s
		}
		return s, nil
	}
	var index struct {
		Manifests []archiveDescriptor `json:"manifests"`
	}
	if err := readJSON(entries["index.json"], &index); err != nil {
		return nil, err
	}
	if len(index.Manifests) != 1 || index.Manifests[0].Digest != sbom.Source.Metadata.ManifestDigest {
		return nil, fmt.Errorf("SBOM and native OCI manifest differ")
	}
	manifestSection, err := blob(index.Manifests[0])
	if err != nil {
		return nil, err
	}
	var manifest struct {
		Config archiveDescriptor   `json:"config"`
		Layers []archiveDescriptor `json:"layers"`
	}
	if err := readJSON(manifestSection, &manifest); err != nil {
		return nil, err
	}
	if manifest.Config.Digest != sbom.Source.Metadata.ImageID {
		return nil, fmt.Errorf("SBOM and OCI config differ")
	}
	configSection, err := blob(manifest.Config)
	if err != nil {
		return nil, err
	}
	var config struct {
		Architecture string `json:"architecture"`
		OS           string `json:"os"`
		Config       struct {
			Labels map[string]string `json:"Labels"`
		} `json:"config"`
	}
	if err := readJSON(configSection, &config); err != nil {
		return nil, err
	}
	meta := sbom.Source.Metadata
	if config.Architecture != meta.Architecture || config.OS != meta.OS || config.Config.Labels["org.opencontainers.image.title"] != meta.Labels["org.opencontainers.image.title"] {
		return nil, fmt.Errorf("SBOM and OCI platform or image owner differ")
	}
	if len(manifest.Layers) == 0 || len(manifest.Layers) > 1024 {
		return nil, fmt.Errorf("invalid OCI layer count")
	}
	result := &Inventory{ManifestDigest: index.Manifests[0].Digest, Files: map[string]inventoryFile{}}
	for _, layer := range manifest.Layers {
		s, err := blob(layer)
		if err != nil {
			return nil, err
		}
		reader := io.Reader(io.NewSectionReader(f, s.offset, s.size))
		var compressed *gzip.Reader
		switch layer.MediaType {
		case "application/vnd.oci.image.layer.v1.tar", "application/vnd.docker.image.rootfs.diff.tar":
		case "application/vnd.oci.image.layer.v1.tar+gzip", "application/vnd.docker.image.rootfs.diff.tar.gzip":
			compressed, err = gzip.NewReader(reader)
			if err != nil {
				return nil, err
			}
			reader = compressed
		default:
			return nil, fmt.Errorf("unsupported OCI layer compression")
		}
		err = applyInventoryLayer(result.Files, reader)
		if compressed != nil {
			closeErr := compressed.Close()
			if err == nil {
				err = closeErr
			}
		}
		if err != nil {
			return nil, err
		}
	}
	for name := range result.Files {
		for parent := path.Dir(name); parent != "/"; parent = path.Dir(parent) {
			if node, exists := result.Files[parent]; exists && node.Kind != "Directory" {
				return nil, fmt.Errorf("ambiguous OCI path beneath non-directory")
			}
		}
	}
	return result, nil
}

func zeroPadding(reader io.Reader) error {
	buffer := make([]byte, 32<<10)
	for {
		n, err := reader.Read(buffer)
		for _, b := range buffer[:n] {
			if b != 0 {
				return fmt.Errorf("nonzero data after tar end marker")
			}
		}
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
	}
}

func inventoryPath(name string) (string, error) {
	if strings.Contains("/"+name+"/", "/../") || strings.ContainsRune(name, 0) || !utf8.ValidString(name) {
		return "", fmt.Errorf("unsafe OCI layer path")
	}
	return path.Clean("/" + strings.TrimPrefix(name, "/")), nil
}

func applyInventoryLayer(files map[string]inventoryFile, reader io.Reader) error {
	tr := tar.NewReader(reader)
	changes := map[string]inventoryFile{}
	removed := []string{}
	opaque := []string{}
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		name, err := inventoryPath(h.Name)
		if err != nil {
			return err
		}
		base := path.Base(name)
		if strings.HasPrefix(base, ".wh.") && (h.Size != 0 || (h.Typeflag != tar.TypeReg && h.Typeflag != tar.TypeRegA)) {
			return fmt.Errorf("invalid OCI whiteout")
		}
		if base == ".wh..wh..opq" {
			opaque = append(opaque, path.Dir(name))
			continue
		}
		if strings.HasPrefix(base, ".wh.") {
			target := strings.TrimPrefix(base, ".wh.")
			if target == "" || target == "." || target == ".." {
				return fmt.Errorf("invalid OCI whiteout target")
			}
			removed = append(removed, path.Join(path.Dir(name), target))
			continue
		}
		if _, ok := changes[name]; ok {
			return fmt.Errorf("duplicate path in OCI layer")
		}
		node := inventoryFile{Mode: h.Mode, UID: h.Uid, GID: h.Gid}
		switch h.Typeflag {
		case tar.TypeReg, tar.TypeRegA:
			hash := sha256.New()
			if _, err := io.Copy(hash, tr); err != nil {
				return err
			}
			node.Kind = "RegularFile"
			node.SHA256 = fmt.Sprintf("%x", hash.Sum(nil))
		case tar.TypeDir:
			node.Kind = "Directory"
		case tar.TypeSymlink:
			if !utf8.ValidString(h.Linkname) {
				return fmt.Errorf("invalid OCI link encoding")
			}
			node.Kind = "SymbolicLink"
			node.Link = h.Linkname
		case tar.TypeLink:
			target, err := inventoryPath(h.Linkname)
			if err != nil {
				return err
			}
			node.Kind = "HardLink"
			node.Link = target
		case tar.TypeChar, tar.TypeBlock, tar.TypeFifo:
			node.Kind = "SpecialFile"
		default:
			return fmt.Errorf("unsupported OCI layer entry")
		}
		changes[name] = node
	}
	// Consume any remaining compressed stream so gzip checksums/truncation errors
	// are observed even when the tar reader stopped at its end marker.
	if err := zeroPadding(reader); err != nil {
		return err
	}
	for name := range files {
		for _, target := range removed {
			if name == target || strings.HasPrefix(name, target+"/") {
				delete(files, name)
			}
		}
		for _, dir := range opaque {
			if strings.HasPrefix(name, strings.TrimSuffix(dir, "/")+"/") {
				delete(files, name)
			}
		}
	}
	for name, node := range changes {
		if node.Kind != "HardLink" {
			files[name] = node
		}
	}
	for pending := true; pending; {
		pending = false
		progress := false
		for name, node := range changes {
			if node.Kind != "HardLink" {
				continue
			}
			pending = true
			if target, ok := changes[node.Link]; ok && target.Kind == "HardLink" {
				continue
			}
			target, ok := files[node.Link]
			if !ok || target.Kind != "RegularFile" {
				continue
			}
			node.Kind = "RegularFile"
			node.SHA256 = target.SHA256
			node.Mode, node.UID, node.GID = target.Mode, target.UID, target.GID
			files[name] = node
			changes[name] = node
			progress = true
		}
		if pending && !progress {
			return fmt.Errorf("unresolved OCI hard link")
		}
	}
	return nil
}
