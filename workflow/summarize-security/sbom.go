package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"syscall"
)

const (
	maxSBOM      = 512 << 20
	maxSBOMToken = 1 << 20
	maxSBOMDepth = 256
)

// Bound decoder buffering even for a malicious enormous string, number or
// whitespace run. JSON syntax validation remains the standard decoder's job.
type boundedTokens struct {
	r               io.Reader
	quoted, escaped bool
	run             int
}

func (r *boundedTokens) Read(p []byte) (int, error) {
	n, err := r.r.Read(p)
	for i, c := range p[:n] {
		r.run++
		if r.run > maxSBOMToken {
			return i, fmt.Errorf("SBOM token or whitespace span exceeds 1 MiB")
		}
		if r.quoted {
			if r.escaped {
				r.escaped = false
			} else if c == '\\' {
				r.escaped = true
			} else if c == '"' {
				r.quoted = false
				r.run = 0
			}
		} else {
			switch c {
			case '"':
				r.quoted = true
				r.run = 0
			case '{', '}', '[', ']', ',', ':':
				r.run = 0
			}
		}
	}
	return n, err
}

// Walk and discard payload tokens rather than retaining packages, files or
// relationships. Only the three top-level format discriminators are retained.
func sbomValue(d *json.Decoder, depth int) (json.Token, error) {
	if depth > maxSBOMDepth {
		return nil, fmt.Errorf("SBOM nesting exceeds %d", maxSBOMDepth)
	}
	first, err := d.Token()
	if err != nil {
		return nil, err
	}
	delim, container := first.(json.Delim)
	if !container {
		return first, nil
	}
	if delim != '{' && delim != '[' {
		return nil, fmt.Errorf("unexpected JSON delimiter")
	}
	for d.More() {
		if delim == '{' {
			if _, err := d.Token(); err != nil {
				return nil, err
			}
		}
		if _, err := sbomValue(d, depth+1); err != nil {
			return nil, err
		}
	}
	_, err = d.Token()
	return first, err
}

func readSBOM(path string) error {
	// NOFOLLOW rejects a final symlink atomically; NONBLOCK prevents FIFO hangs.
	fd, err := syscall.Open(path, syscall.O_RDONLY|syscall.O_CLOEXEC|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if err != nil {
		return err
	}
	f := os.NewFile(uintptr(fd), path)
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Size() > maxSBOM {
		return fmt.Errorf("SBOM must be a regular file of at most 512 MiB")
	}
	limited := &io.LimitedReader{R: f, N: maxSBOM + 1}
	d := json.NewDecoder(&boundedTokens{r: limited})
	d.UseNumber()
	first, err := d.Token()
	if err != nil {
		return err
	}
	if first != json.Delim('{') {
		return fmt.Errorf("SBOM must be a JSON object")
	}
	var spdx, cyclone, syft bool
	for d.More() {
		key, err := d.Token()
		if err != nil {
			return err
		}
		value, err := sbomValue(d, 1)
		if err != nil {
			return err
		}
		switch key {
		case "spdxVersion":
			s, _ := value.(string)
			spdx = strings.HasPrefix(s, "SPDX-")
		case "bomFormat":
			cyclone = value == "CycloneDX"
		case "artifacts":
			syft = value == json.Delim('[')
		}
	}
	if _, err := d.Token(); err != nil {
		return err
	}
	// Validate the entire document, including its tail and a growing input.
	_, err = d.Token()
	if limited.N == 0 {
		return fmt.Errorf("SBOM exceeds 512 MiB")
	}
	if err != io.EOF {
		return fmt.Errorf("invalid SBOM trailing data: %v", err)
	}
	if !spdx && !cyclone && !syft {
		return fmt.Errorf("unrecognized SBOM document")
	}
	return nil
}
