package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type rpmIdentity struct{ Name, Epoch, VersionRelease, Architecture string }
type coverage struct {
	Database        string `json:"database"`
	Installed       int    `json:"installed"`
	Covered         int    `json:"covered"`
	SBOMRPMs        int    `json:"sbom_rpm_packages"`
	Additional      int    `json:"additional_rpm_packages"`
	InventorySHA256 string `json:"inventory_sha256"`
}
type spdxPackage struct {
	Name         string `json:"name"`
	Version      string `json:"versionInfo"`
	ExternalRefs []struct {
		Type  string `json:"referenceType"`
		Value string `json:"referenceLocator"`
	} `json:"externalRefs"`
}
type spdx struct {
	Version   string `json:"spdxVersion"`
	ID        string `json:"SPDXID"`
	License   string `json:"dataLicense"`
	Name      string `json:"name"`
	Namespace string `json:"documentNamespace"`
	Creation  struct {
		Creators []string `json:"creators"`
		Created  string   `json:"created"`
	} `json:"creationInfo"`
	Packages []spdxPackage `json:"packages"`
}

func copyRPMDatabase(rootfs, work string) (string, error) {
	root, err := os.OpenRoot(rootfs)
	if err != nil {
		return "", err
	}
	defer root.Close()
	for _, name := range []string{"usr/share/rpm", "usr/lib/sysimage/rpm", "var/lib/rpm"} {
		db, err := root.OpenRoot(name)
		if err != nil {
			continue
		}
		valid := false
		for _, marker := range []string{"rpmdb.sqlite", "Packages", "Packages.db"} {
			info, e := db.Lstat(marker)
			if e == nil && info.Mode().IsRegular() {
				valid = true
			}
		}
		if !valid {
			db.Close()
			continue
		}
		dest := filepath.Join(work, "rpmdb-query")
		if err = os.Mkdir(dest, 0700); err != nil {
			db.Close()
			return "", err
		}
		dir, e := db.Open(".")
		if e != nil {
			db.Close()
			return "", e
		}
		entries, e := dir.ReadDir(-1)
		dir.Close()
		if e != nil {
			db.Close()
			return "", e
		}
		for _, entry := range entries {
			info, e := db.Lstat(entry.Name())
			if e != nil {
				db.Close()
				return "", e
			}
			if !info.Mode().IsRegular() {
				db.Close()
				return "", errors.New("RPM database contains nonregular members")
			}
			src, e := db.Open(entry.Name())
			if e != nil {
				db.Close()
				return "", e
			}
			dst, e := os.OpenFile(filepath.Join(dest, entry.Name()), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
			if e != nil {
				src.Close()
				db.Close()
				return "", e
			}
			_, copyErr := io.Copy(dst, src)
			closeErr := errors.Join(src.Close(), dst.Close())
			if e = errors.Join(copyErr, closeErr); e != nil {
				db.Close()
				return "", e
			}
		}
		db.Close()
		return name, nil
	}
	return "", errors.New("Fedora/RPM Atomic source must contain a readable RPM database")
}
func inventory(raw []byte) (map[rpmIdentity]int, string, error) {
	entries := map[rpmIdentity]int{}
	lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
	for _, line := range lines {
		fields := strings.Split(line, "|")
		if len(fields) != 5 || fields[0] == "" || fields[2] == "" || fields[3] == "" {
			return nil, "", errors.New("invalid static RPM inventory")
		}
		arch := fields[4]
		if arch == "(none)" {
			arch = ""
		}
		epoch := fields[1]
		if epoch == "(none)" || epoch == "" {
			epoch = "0"
		}
		entries[rpmIdentity{fields[0], epoch, fields[2] + "-" + fields[3], arch}]++
	}
	sort.Strings(lines)
	sum := sha256.Sum256([]byte(strings.Join(lines, "\n") + "\n"))
	return entries, hex.EncodeToString(sum[:]), nil
}
func verifyCoverage(path string, installed map[rpmIdentity]int) (coverage, int, error) {
	f, err := os.Open(path)
	if err != nil {
		return coverage{}, 0, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return coverage{}, 0, err
	}
	if info.Size() <= 0 || info.Size() > 1<<30 {
		return coverage{}, 0, errors.New("empty or oversized SBOM")
	}
	var doc spdx
	dec := json.NewDecoder(f)
	if err = dec.Decode(&doc); err != nil {
		return coverage{}, 0, fmt.Errorf("invalid SPDX JSON: %w", err)
	}
	var trailing any
	if err = dec.Decode(&trailing); err != io.EOF {
		return coverage{}, 0, errors.New("trailing SBOM data")
	}
	if doc.Version != "SPDX-2.3" || len(doc.Packages) == 0 {
		return coverage{}, 0, errors.New("complete SPDX 2.3 package document required")
	}
	if doc.ID != "SPDXRef-DOCUMENT" || doc.License != "CC0-1.0" || doc.Name == "" || len(doc.Creation.Creators) == 0 {
		return coverage{}, 0, errors.New("missing SPDX document metadata")
	}
	if u, e := url.Parse(doc.Namespace); e != nil || u.Scheme == "" {
		return coverage{}, 0, errors.New("invalid SPDX document namespace")
	}
	if _, e := time.Parse(time.RFC3339, doc.Creation.Created); e != nil {
		return coverage{}, 0, errors.New("invalid SPDX creation time")
	}
	reported := map[rpmIdentity]int{}
	for _, p := range doc.Packages {
		for _, ref := range p.ExternalRefs {
			if ref.Type != "purl" || !strings.HasPrefix(ref.Value, "pkg:rpm/") {
				continue
			}
			u, e := url.Parse(ref.Value)
			if e != nil {
				return coverage{}, 0, e
			}
			nv := u.Opaque
			if nv == "" {
				nv = u.Path
			}
			// pkg: is an opaque URI. Parse the PURL query separately.
			nv, query, _ := strings.Cut(nv, "?")
			if query == "" {
				query = u.RawQuery
			}
			params, e := url.ParseQuery(query)
			if e != nil {
				return coverage{}, 0, e
			}
			at := strings.LastIndexByte(nv, '@')
			slash := strings.LastIndexByte(nv[:max(at, 0)], '/')
			if at < 0 || slash < 0 {
				return coverage{}, 0, errors.New("invalid RPM PURL")
			}
			name, e := url.PathUnescape(nv[slash+1 : at])
			if e != nil {
				return coverage{}, 0, e
			}
			version, e := url.PathUnescape(nv[at+1:])
			if e != nil {
				return coverage{}, 0, e
			}
			epoch := params.Get("epoch")
			if epoch == "" {
				epoch = "0"
			}
			reported[rpmIdentity{name, epoch, version, params.Get("arch")}]++
		}
	}
	c := coverage{}
	for _, count := range reported {
		c.SBOMRPMs += count
	}
	for id, count := range installed {
		c.Installed += count
		if reported[id] < count {
			return c, len(doc.Packages), errors.New("SBOM is missing an installed RPM identity")
		}
		c.Covered += count
	}
	if c.Installed == 0 {
		return c, len(doc.Packages), errors.New("empty RPM inventory")
	}
	c.Additional = c.SBOMRPMs - c.Installed
	return c, len(doc.Packages), nil
}

func queryRPM(ctx context.Context, work string, run runner) (map[rpmIdentity]int, string, error) {
	result, err := run(ctx, command{Name: "rpm", Work: work, Capture: true, Args: []string{"--dbpath", filepath.Join(work, "rpmdb-query"),
		"-qa", "--qf", "%{NAME}|%{EPOCHNUM}|%{VERSION}|%{RELEASE}|%{ARCH}\n"}})
	if err != nil {
		return nil, "", err
	}
	return inventory(result.Output)
}
