package review

import (
	"regexp"
	"sort"
	"strings"
)

// Backport binds a finite reviewed advisory set to exact native payload bytes.
// A package version alone is never sufficient evidence of a local backport.
type Backport struct {
	Namespace  string                       `json:"namespace"`
	Codename   string                       `json:"codename"`
	Version    string                       `json:"version"`
	Packages   []string                     `json:"packages"`
	Advisories map[string]string            `json:"advisories"`
	Sources    map[string]string            `json:"source_sha256"`
	Variants   map[string]map[string]string `json:"variants"`
	Reason     string                       `json:"reason"`
}

func backportReview(sbom SBOM, p Backport) ([]Rule, []map[string]any) {
	sha := regexp.MustCompile(`^[a-f0-9]{64}$`)
	if sbom.Distro.ID != "debian" || sbom.Distro.Codename != p.Codename || p.Codename != "trixie" || p.Namespace != "debian:distro:debian:13" || p.Version == "" || p.Reason == "" || len(p.Packages) == 0 || len(p.Advisories) == 0 || len(p.Sources) == 0 {
		return nil, nil
	}
	for reference, sum := range p.Sources {
		if !strings.HasPrefix(reference, "https://") || !sha.MatchString(sum) {
			return nil, nil
		}
	}
	wanted := map[string]bool{}
	for _, name := range p.Packages {
		if name == "" || wanted[name] {
			return nil, nil
		}
		wanted[name] = true
	}
	found := map[string]string{}
	architecture := ""
	for _, artifact := range sbom.Artifacts {
		if !wanted[artifact.Name] || artifact.Type != "deb" {
			continue
		}
		if artifact.Version != p.Version || found[artifact.Name] != "" || artifact.ID == "" {
			return nil, nil
		}
		arch := artifact.Metadata.Architecture
		if arch == "" {
			return nil, nil
		}
		if arch != "all" {
			if architecture == "" {
				architecture = arch
			}
			if architecture != arch {
				return nil, nil
			}
		}
		found[artifact.Name] = artifact.ID
	}
	if len(found) != len(wanted) {
		return nil, nil
	}
	if architecture == "" {
		architecture = "all"
	}
	expected := p.Variants[architecture]
	if len(expected) == 0 {
		return nil, nil
	}
	files := map[string][]File{}
	for _, file := range sbom.Files {
		files[file.Location.Path] = append(files[file.Location.Path], file)
	}
	for name, hash := range expected {
		if !strings.HasPrefix(name, "/") || strings.Contains(name, "..") || !sha.MatchString(hash) || len(files[name]) != 1 {
			return nil, nil
		}
		count := 0
		for _, digest := range files[name][0].Digests {
			if digest.Algorithm == "sha256" {
				count++
				if digest.Value != hash {
					return nil, nil
				}
			}
		}
		if count != 1 {
			return nil, nil
		}
	}
	ids := []string{}
	for id, reference := range p.Advisories {
		if !regexp.MustCompile(`^CVE-[0-9]{4}-[0-9]{4,}$`).MatchString(id) || !strings.HasPrefix(reference, "https://") {
			return nil, nil
		}
		ids = append(ids, id)
	}
	sort.Strings(ids)
	rules := []Rule{}
	for _, id := range ids {
		for _, name := range p.Packages {
			// Grype interprets metacharacters such as '+' as a package-name regex.
			rules = append(rules, Rule{id, p.Namespace, map[string]string{"name": regexp.QuoteMeta(name), "version": p.Version, "type": "deb"}, p.Reason})
		}
	}
	return rules, []map[string]any{{"packages": found, "version": p.Version, "architecture": architecture, "verified_files": expected, "source_sha256": p.Sources, "advisories": p.Advisories}}
}
