package rustgtk

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const InstallREADME = `# Binary installation

Requires Fedora 44-compatible glibc and GTK 4 (4.22 recommended). Extract this
archive into a prefix such as ~/.local; put its bin directory on PATH. Copy
share/doc/niri-tonic-files/config.example.yaml to your configuration location
only after reviewing it. No personal configuration or credentials are included.

Plugin sources live in share/niri-tonic-files/plugins and are discovered automatically
when bin and share are installed under the same prefix. Custom installations may
configure absolute adapter.mjs paths. Both need an external Node.js >=22
runtime. Filen additionally requires running npm ci --omit=dev --ignore-scripts in its extracted
plugin directory (requires network access). Google Drive has no npm runtime
dependencies. Authentication is performed by the user, never during qualification.

The included Cargo.lock describes source dependencies; its package version remains
the Cargo source version. The compiled release version is reported by --version.
`

func sourceGit(root string, args ...string) ([]byte, error) {
	cmd := exec.Command("git", append([]string{"-c", "safe.directory=" + root}, args...)...)
	cmd.Dir = root
	cmd.Stderr = os.Stderr
	return cmd.Output()
}

// Package writes only explicit public resources, never recursive checkout data.
func Package(root, binary, dest string, epoch int64) error {
	files := map[string]string{"bin/" + Component: binary, "share/applications/io.github.tdesaules.NiriTonicFiles.desktop": "data/io.github.tdesaules.NiriTonicFiles.desktop", "share/icons/hicolor/scalable/apps/io.github.tdesaules.NiriTonicFiles.svg": "data/io.github.tdesaules.NiriTonicFiles.svg", "share/licenses/" + Component + "/LICENSE": "LICENSE", "share/doc/" + Component + "/README.md": "README.md", "share/doc/" + Component + "/config.example.yaml": "config.example.yaml", "share/doc/" + Component + "/Cargo.lock": "Cargo.lock"}
	// Resolve names and immutable blob IDs from the committed tree. Never package
	// untracked public-looking files or read resource contents from the worktree.
	tree, err := sourceGit(root, "ls-tree", "-rz", "--full-tree", "HEAD")
	if err != nil {
		return err
	}
	blobs := map[string]string{}
	for _, entry := range strings.Split(strings.TrimSuffix(string(tree), "\x00"), "\x00") {
		meta, rel, ok := strings.Cut(entry, "\t")
		fields := strings.Fields(meta)
		if !ok || len(fields) != 3 {
			return fmt.Errorf("invalid source tree entry")
		}
		if fields[0] != "100644" && fields[0] != "100755" {
			continue
		}
		blobs[rel] = fields[2]
		name, dir := filepath.Base(rel), filepath.Dir(rel)
		if strings.HasPrefix(rel, "docs/") && filepath.Ext(name) == ".md" {
			files["share/doc/"+Component+"/"+rel] = rel
		} else if dir == "plugins/filen" || dir == "plugins/google-drive" {
			if filepath.Ext(name) == ".mjs" || name == "package.json" || name == "package-lock.json" || name == "LICENSE" || name == "README.md" {
				files["share/"+Component+"/"+rel] = rel
			}
		}
	}
	for name, src := range files {
		if name != "bin/"+Component && blobs[src] == "" {
			return fmt.Errorf("package resource must be a committed regular file: %s", src)
		}
	}
	if e := os.MkdirAll(filepath.Dir(dest), 0755); e != nil {
		return e
	}
	f, e := os.Create(dest)
	if e != nil {
		return e
	}
	defer f.Close()
	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)
	names := []string{}
	for name := range files {
		names = append(names, name)
	}
	install := "share/doc/" + Component + "/INSTALL.md"
	names = append(names, install)
	sort.Strings(names)
	for _, name := range names {
		var data []byte
		mode := int64(0644)
		if name == install {
			data = []byte(InstallREADME)
		} else if name != "bin/"+Component {
			data, e = sourceGit(root, "cat-file", "blob", blobs[files[name]])
			if e != nil {
				return e
			}
		} else {
			src := files[name]
			if !filepath.IsAbs(src) {
				src = filepath.Join(root, src)
			}
			info, e := os.Lstat(src)
			if e != nil {
				return e
			}
			if !info.Mode().IsRegular() {
				return fmt.Errorf("nonregular package input %s", src)
			}
			data, e = os.ReadFile(src)
			if e != nil {
				return e
			}
			if strings.HasPrefix(name, "bin/") {
				mode = 0755
			}
		}
		if e = tw.WriteHeader(&tar.Header{Name: name, Mode: mode, Size: int64(len(data)), ModTime: time.Unix(epoch, 0), Typeflag: tar.TypeReg, Format: tar.FormatPAX}); e != nil {
			return e
		}
		if _, e = tw.Write(data); e != nil {
			return e
		}
	}
	if e = tw.Close(); e != nil {
		return e
	}
	if e = gz.Close(); e != nil {
		return e
	}
	return f.Close()
}
func Extract(archive, dest string) error {
	f, e := os.Open(archive)
	if e != nil {
		return e
	}
	defer f.Close()
	gz, e := gzip.NewReader(f)
	if e != nil {
		return e
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	seen := map[string]bool{}
	var total int64
	for {
		h, e := tr.Next()
		if e == io.EOF {
			return nil
		}
		if e != nil {
			return e
		}
		clean := filepath.Clean(h.Name)
		if !filepath.IsLocal(h.Name) || clean != h.Name || seen[clean] || h.Typeflag != tar.TypeReg {
			return fmt.Errorf("unsafe archive member %q", h.Name)
		}
		seen[clean] = true
		total += h.Size
		if h.Size < 0 || total > 2<<30 {
			return fmt.Errorf("archive size limit")
		}
		p := filepath.Join(dest, clean)
		if e = os.MkdirAll(filepath.Dir(p), 0755); e != nil {
			return e
		}
		mode := os.FileMode(0644)
		if strings.HasPrefix(clean, "bin/") {
			mode = 0755
		}
		o, e := os.OpenFile(p, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode)
		if e != nil {
			return e
		}
		_, e = io.Copy(o, tr)
		ce := o.Close()
		if e != nil {
			return e
		}
		if ce != nil {
			return ce
		}
	}
}
