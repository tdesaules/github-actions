package main

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

func checkDestination(root *os.Root, path string) error {
	if _, err := root.Lstat(path); !os.IsNotExist(err) {
		return errors.New("output already exists or cannot be checked; choose fresh SBOM and binding paths")
	}
	parent := filepath.Dir(path)
	for parent != "." {
		if info, err := root.Lstat(parent); err == nil {
			if !info.IsDir() {
				return errors.New("output ancestors must be real workspace directories")
			}
		} else if !os.IsNotExist(err) {
			return err
		}
		parent = filepath.Dir(parent)
	}
	return nil
}
func outputOwner(workspace string) (int, int, error) {
	path := os.Getenv("GITHUB_OUTPUT")
	if path == "" {
		path = workspace
	}
	info, err := os.Stat(path)
	if err != nil {
		return 0, 0, err
	}
	stat := info.Sys().(*syscall.Stat_t)
	return int(stat.Uid), int(stat.Gid), nil
}
func makeOutputParents(root *os.Root, name string, uid, gid int) error {
	dir := ""
	for _, part := range strings.Split(filepath.Dir(name), string(filepath.Separator)) {
		if part == "." {
			continue
		}
		dir = filepath.Join(dir, part)
		err := root.Mkdir(dir, 0755)
		if os.IsExist(err) {
			info, e := root.Lstat(dir)
			if e != nil {
				return e
			}
			if !info.IsDir() {
				return errors.New("unsafe output ancestor")
			}
			continue
		}
		if err != nil {
			return err
		}
		if os.Geteuid() == 0 {
			f, e := root.Open(dir)
			if e != nil {
				return e
			}
			e = errors.Join(f.Chown(uid, gid), f.Close())
			if e != nil {
				return e
			}
		}
	}
	return nil
}
func publish(ctx context.Context, root *os.Root, workspace string, o options, source io.Reader, expected string, report []byte) (err error) {
	uid, gid, err := outputOwner(workspace)
	if err != nil {
		return err
	}
	owned := map[string]*os.File{}
	defer func() {
		for _, f := range owned {
			err = errors.Join(err, f.Close())
		}
		if err != nil {
			for name := range owned {
				_ = root.Remove(name)
			}
		}
	}()
	for _, name := range []string{o.Output, o.Binding} {
		if err = makeOutputParents(root, name, uid, gid); err != nil {
			return err
		}
		f, e := root.OpenFile(name, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
		if e != nil {
			return e
		}
		owned[name] = f
		if os.Geteuid() == 0 {
			if e = f.Chown(uid, gid); e != nil {
				return e
			}
		}
	}
	h := sha256.New()
	if _, err = io.Copy(io.MultiWriter(owned[o.Output], h), contextReader{ctx, source}); err != nil {
		return err
	}
	if fmt.Sprintf("%x", h.Sum(nil)) != expected {
		return errors.New("SBOM changed during publication")
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	_, err = owned[o.Binding].Write(report)
	return err
}
