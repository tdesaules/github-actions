package main

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

type contextReader struct {
	ctx context.Context
	io.Reader
}

func (r contextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.Reader.Read(p)
}

// Reserve all three outputs exclusively before copying. Root confines symlink
// resolution, and rollback removes only files this invocation actually created.
func publishISO(ctx context.Context, source, output, expected string, receipt []byte) (err error) {
	root, err := os.OpenRoot(".")
	if err != nil {
		return err
	}
	defer root.Close()
	if err = root.MkdirAll(filepath.Dir(output), 0755); err != nil {
		return err
	}
	files := map[string]*os.File{}
	defer func() {
		for _, f := range files {
			err = errors.Join(err, f.Close())
		}
		if err != nil {
			for name := range files {
				_ = root.Remove(name)
			}
		}
	}()
	for _, name := range []string{output, output + "-CHECKSUM", output + ".build.json"} {
		f, e := root.OpenFile(name, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
		if e != nil {
			return e
		}
		files[name] = f
	}
	src, err := os.Open(source)
	if err != nil {
		return err
	}
	defer src.Close()
	h := sha256.New()
	if _, err = io.Copy(io.MultiWriter(files[output], h), contextReader{ctx, src}); err != nil {
		return err
	}
	if fmt.Sprintf("%x", h.Sum(nil)) != expected {
		return fmt.Errorf("ISO changed during output copy")
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	if _, err = fmt.Fprintf(files[output+"-CHECKSUM"], "%s  %s\n", expected, filepath.Base(output)); err != nil {
		return err
	}
	_, err = files[output+".build.json"].Write(receipt)
	return err
}
