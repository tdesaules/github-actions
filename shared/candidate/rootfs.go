// Package candidate inspects and executes native OCI candidates without an engine socket.
package candidate

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"syscall"
	"time"
)

type Descriptor struct {
	Digest    string `json:"digest"`
	Size      int64  `json:"size"`
	MediaType string `json:"mediaType"`
}
type Config struct {
	Architecture string `json:"architecture"`
	OS           string `json:"os"`
	Config       struct {
		Entrypoint []string          `json:"Entrypoint"`
		Env        []string          `json:"Env"`
		Labels     map[string]string `json:"Labels"`
	} `json:"config"`
}
type Root struct {
	Path   string
	Config Config
}

var digestPattern = regexp.MustCompile(`^sha256:[a-f0-9]{64}$`)

func blob(layout string, d Descriptor) (string, error) {
	if !digestPattern.MatchString(d.Digest) {
		return "", fmt.Errorf("invalid OCI digest")
	}
	p := filepath.Join(layout, "blobs/sha256", strings.TrimPrefix(d.Digest, "sha256:"))
	f, err := os.Open(p)
	if err != nil {
		return "", err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() || info.Size() != d.Size {
		return "", fmt.Errorf("invalid OCI blob size or type")
	}
	h := sha256.New()
	if _, err = io.Copy(h, f); err != nil {
		return "", err
	}
	if fmt.Sprintf("sha256:%x", h.Sum(nil)) != d.Digest {
		return "", fmt.Errorf("OCI blob checksum mismatch")
	}
	return p, nil
}
func metadata(layout string, d Descriptor, v any) error {
	p, err := blob(layout, d)
	if err != nil {
		return err
	}
	if d.Size > 16<<20 {
		return fmt.Errorf("oversized OCI metadata")
	}
	data, err := os.ReadFile(p)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, v)
}

func Open(layout, root, image, arch, revision string) (*Root, error) {
	if arch != runtime.GOARCH {
		return nil, fmt.Errorf("candidate must execute on its native architecture")
	}
	data, err := os.ReadFile(filepath.Join(layout, "index.json"))
	if err != nil {
		return nil, err
	}
	var index struct {
		Manifests []Descriptor `json:"manifests"`
	}
	if err = json.Unmarshal(data, &index); err != nil {
		return nil, err
	}
	if len(index.Manifests) != 1 {
		return nil, fmt.Errorf("one native image required")
	}
	var manifest struct {
		Config Descriptor   `json:"config"`
		Layers []Descriptor `json:"layers"`
	}
	if err = metadata(layout, index.Manifests[0], &manifest); err != nil {
		return nil, err
	}
	r := &Root{Path: root}
	if err = metadata(layout, manifest.Config, &r.Config); err != nil {
		return nil, err
	}
	if r.Config.OS != "linux" || r.Config.Architecture != arch || len(r.Config.Config.Entrypoint) != 1 || r.Config.Config.Entrypoint[0] != "/entrypoint" {
		return nil, fmt.Errorf("unexpected platform or entrypoint")
	}
	if r.Config.Config.Labels["org.opencontainers.image.title"] != image {
		return nil, fmt.Errorf("candidate image owner mismatch")
	}
	if revision != "" && r.Config.Config.Labels["org.opencontainers.image.revision"] != revision {
		return nil, fmt.Errorf("candidate revision mismatch")
	}
	if err = os.MkdirAll(root, 0755); err != nil {
		return nil, err
	}
	for _, layer := range manifest.Layers {
		p, err := blob(layout, layer)
		if err != nil {
			return nil, err
		}
		if err = extract(root, p, layer.MediaType); err != nil {
			return nil, err
		}
	}
	info, err := os.Lstat(filepath.Join(root, "tmp"))
	if err != nil {
		return nil, err
	}
	if !info.IsDir() || info.Mode().Perm() != 0777 || info.Mode()&os.ModeSticky == 0 {
		return nil, fmt.Errorf("candidate /tmp must be 01777")
	}
	for _, name := range []string{"github", "tmp", "proc", "sys", "dev"} {
		p := filepath.Join(root, name)
		err := filepath.WalkDir(p, func(_ string, e fs.DirEntry, walkErr error) error {
			if os.IsNotExist(walkErr) {
				return nil
			}
			if walkErr != nil {
				return walkErr
			}
			if !e.IsDir() {
				return fmt.Errorf("candidate contains data in runner-provided directory /%s", name)
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	return r, nil
}

func local(name string) (string, error) {
	name = path.Clean(strings.TrimPrefix(name, "/"))
	if name == ".." || strings.HasPrefix(name, "../") || strings.ContainsRune(name, 0) {
		return "", fmt.Errorf("unsafe archive path")
	}
	return name, nil
}
func extract(root, layer, media string) error {
	f, err := os.Open(layer)
	if err != nil {
		return err
	}
	defer f.Close()
	var reader io.Reader = f
	if strings.HasSuffix(media, "+gzip") || strings.HasSuffix(media, ".gzip") {
		gz, err := gzip.NewReader(f)
		if err != nil {
			return err
		}
		defer gz.Close()
		reader = gz
	} else if media != "application/vnd.oci.image.layer.v1.tar" && media != "application/vnd.docker.image.rootfs.diff.tar" {
		return fmt.Errorf("unsupported layer compression %s", media)
	}
	scope, err := os.OpenRoot(root)
	if err != nil {
		return err
	}
	defer scope.Close()
	tr := tar.NewReader(reader)
	for {
		h, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		name, err := local(h.Name)
		if err != nil {
			return err
		}
		if name == "." {
			continue
		}
		base, parent := path.Base(name), path.Dir(name)
		if base == ".wh..wh..opq" {
			entries, err := fs.ReadDir(scope.FS(), parent)
			if os.IsNotExist(err) {
				continue
			}
			if err != nil {
				return err
			}
			for _, e := range entries {
				if err = scope.RemoveAll(path.Join(parent, e.Name())); err != nil {
					return err
				}
			}
			continue
		}
		if strings.HasPrefix(base, ".wh.") {
			if err = scope.RemoveAll(path.Join(parent, strings.TrimPrefix(base, ".wh."))); err != nil {
				return err
			}
			continue
		}
		if err = scope.MkdirAll(parent, 0755); err != nil {
			return err
		}
		mode := os.FileMode(h.Mode & 0777)
		if h.Mode&01000 != 0 {
			mode |= os.ModeSticky
		}
		switch h.Typeflag {
		case tar.TypeDir:
			if err = scope.MkdirAll(name, mode.Perm()); err != nil {
				return err
			}
			if err = scope.Chmod(name, mode); err != nil {
				return err
			}
		case tar.TypeReg:
			if err = scope.RemoveAll(name); err != nil {
				return err
			}
			out, err := scope.OpenFile(name, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode)
			if err != nil {
				return err
			}
			_, copyErr := io.Copy(out, tr)
			closeErr := out.Close()
			if copyErr != nil {
				return copyErr
			}
			if closeErr != nil {
				return closeErr
			}
		case tar.TypeSymlink:
			target := h.Linkname
			if !path.IsAbs(target) {
				target = path.Join(parent, target)
			}
			target, err = local(target)
			if err != nil {
				return err
			}
			relative, err := filepath.Rel(parent, target)
			if err != nil {
				return err
			}
			if err = scope.RemoveAll(name); err != nil {
				return err
			}
			if err = scope.Symlink(relative, name); err != nil {
				return err
			}
		case tar.TypeLink:
			target, err := local(h.Linkname)
			if err != nil {
				return err
			}
			if err = scope.RemoveAll(name); err != nil {
				return err
			}
			if err = scope.Link(target, name); err != nil {
				return err
			}
		default:
			return fmt.Errorf("unsupported archive file type for %s", name)
		}
	}
}

// Protect makes root-owned image files immutable to the unprivileged child.
// /tmp and /reports are explicit writable test scratch areas, never source mounts.
func (r *Root) Protect() error {
	for _, dir := range []string{"fixtures", "reports"} {
		if err := os.MkdirAll(filepath.Join(r.Path, dir), 0755); err != nil {
			return err
		}
	}
	return filepath.WalkDir(r.Path, func(p string, e fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if e.Type()&os.ModeSymlink != 0 {
			return nil
		}
		relative, err := filepath.Rel(r.Path, p)
		if err != nil {
			return err
		}
		if relative == "tmp" || relative == "reports" {
			if err = os.Chmod(p, 0777|os.ModeSticky); err != nil {
				return err
			}
			return filepath.SkipDir
		}
		info, err := e.Info()
		if err != nil {
			return err
		}
		mode := info.Mode().Perm() & 0555
		if e.IsDir() {
			mode = 0555
		}
		return os.Chmod(p, mode)
	})
}

// Install materializes the candidate in this disposable qualification container.
// Runtime mounts (/dev, /proc, workspace, DNS) remain supplied by the runner.
// No nested engine, mount syscall, mknod or additional capability is needed.
func (r *Root) Install() error {
	marker, err := os.ReadFile("/.candidate-runtime")
	if err != nil || string(marker) != "disposable-candidate-runtime\n" || os.Geteuid() != 0 {
		return fmt.Errorf("candidate installation requires its disposable action image")
	}
	copyTree := func(source, destination string) error {
		return filepath.WalkDir(source, func(p string, e fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			relative, err := filepath.Rel(source, p)
			if err != nil {
				return err
			}
			target := filepath.Join(destination, relative)
			info, err := e.Info()
			if err != nil {
				return err
			}
			if e.IsDir() {
				if err = os.MkdirAll(target, 0755); err != nil {
					return err
				}
				return os.Chmod(target, info.Mode())
			}
			if err = os.RemoveAll(target); err != nil {
				return err
			}
			if e.Type()&os.ModeSymlink != 0 {
				link, err := os.Readlink(p)
				if err != nil {
					return err
				}
				return os.Symlink(link, target)
			}
			if !info.Mode().IsRegular() {
				return fmt.Errorf("unsupported runtime entry %s", p)
			}
			in, err := os.Open(p)
			if err != nil {
				return err
			}
			defer in.Close()
			out, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, info.Mode())
			if err != nil {
				return err
			}
			_, err = io.Copy(out, in)
			return errors.Join(err, out.Close())
		})
	}
	// Remove bootstrap executables/libraries so they cannot mask missing candidate
	// dependencies. Do not traverse or replace any runner-provided mount.
	for _, name := range []string{"usr", "lib", "lib64", "bin", "sbin", "opt"} {
		if err = os.RemoveAll("/" + name); err != nil {
			return err
		}
	}
	if err = os.Chmod("/var/tmp", 0555); err != nil && !os.IsNotExist(err) {
		return err
	}
	entries, err := os.ReadDir("/etc")
	if err != nil {
		return err
	}
	for _, e := range entries {
		if e.Name() == "hosts" || e.Name() == "hostname" || e.Name() == "resolv.conf" {
			continue
		}
		if err = os.RemoveAll(filepath.Join("/etc", e.Name())); err != nil {
			return err
		}
	}
	entries, err = os.ReadDir(r.Path)
	if err != nil {
		return err
	}
	for _, e := range entries {
		name := e.Name()
		source := filepath.Join(r.Path, name)
		switch name {
		case "github", "proc", "sys", "dev", "tmp":
			continue
		case ".candidate-runtime":
			data, err := os.ReadFile(source)
			if err != nil || string(data) != "disposable-candidate-runtime\n" {
				return fmt.Errorf("candidate cannot replace the qualification marker")
			}
			continue
		case "etc", "var", "run":
			children, err := os.ReadDir(source)
			if err != nil {
				return err
			}
			for _, child := range children {
				if name == "etc" && (child.Name() == "hosts" || child.Name() == "hostname" || child.Name() == "resolv.conf") {
					continue
				}
				if (name == "var" && child.Name() == "run") || (name == "run" && child.Name() == "docker.sock") {
					continue
				}
				target := filepath.Join("/", name, child.Name())
				if err = os.RemoveAll(target); err != nil {
					return err
				}
				if err = copyTree(filepath.Join(source, child.Name()), target); err != nil {
					return err
				}
			}
		default:
			target := "/" + name
			if err = os.RemoveAll(target); err != nil {
				return err
			}
			if err = copyTree(source, target); err != nil {
				return err
			}
		}
	}
	r.Path = "/"
	return nil
}

func (r *Root) Run(binary string, args ...string) (string, int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	cmd, err := r.Command(ctx, binary, args...)
	if err != nil {
		return "", -1, err
	}
	data, err := cmd.CombinedOutput()
	if ctx.Err() != nil {
		return string(data), -1, ctx.Err()
	}
	if err == nil {
		return string(data), 0, nil
	}
	var exit *exec.ExitError
	if errors.As(err, &exit) && exit.ExitCode() >= 0 {
		return string(data), exit.ExitCode(), nil
	}
	return string(data), -1, err
}

func (r *Root) Command(ctx context.Context, binary string, args ...string) (*exec.Cmd, error) {
	cmd := exec.CommandContext(ctx, binary, args...)
	cmd.Dir = "/fixtures"
	cmd.Env = append(append([]string{}, r.Config.Config.Env...), "HOME=/tmp", "SYFT_CHECK_FOR_APP_UPDATE=false")
	if r.Path != "/" {
		return nil, fmt.Errorf("candidate runtime is not installed")
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{Credential: &syscall.Credential{Uid: 65532, Gid: 65532}, Setpgid: true}
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
	cmd.WaitDelay = 5 * time.Second
	return cmd, nil
}
