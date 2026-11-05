package review

import (
	"path"
	"regexp"
	"sort"
	"strings"
	"time"
)

// Applicability proves a finite non-applicable advisory set for one image owner.
// It is not a risk-acceptance mechanism: exact payloads and positive source/ABI
// evidence are mandatory, and absent components require a complete inventory.
type Applicability struct {
	ID              string                          `json:"id"`
	Image           string                          `json:"image"`
	Namespace       string                          `json:"namespace"`
	Codename        string                          `json:"codename"`
	Basis           string                          `json:"basis"`
	Owner           string                          `json:"owner"`
	Reviewed        string                          `json:"reviewed"`
	Expires         string                          `json:"expires"`
	Reason          string                          `json:"reason"`
	Advisories      map[string]string               `json:"advisories"`
	Sources         map[string]string               `json:"source_sha256"`
	AbsentFiles     []string                        `json:"absent_file_names,omitempty"`
	AbsentArtifacts []AbsentArtifact                `json:"absent_artifacts,omitempty"`
	Variants        map[string]ApplicabilityVariant `json:"variants"`
}

type AbsentArtifact struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

type ApplicabilityVariant struct {
	InventorySHA256 string            `json:"inventory_sha256"`
	Packages        map[string]string `json:"packages"`
	Files           map[string]string `json:"files"`
	AbsentSHA256    []string          `json:"absent_sha256,omitempty"`
}

func applicabilityReview(sbom SBOM, p Applicability, now time.Time) ([]Rule, []map[string]any) {
	if strings.TrimSpace(p.ID) == "" || strings.TrimSpace(p.Image) == "" || strings.TrimSpace(p.Owner) == "" || strings.TrimSpace(p.Reason) == "" {
		return nil, nil
	}
	reviewed, err := time.Parse("2006-01-02", p.Reviewed)
	if err != nil {
		return nil, nil
	}
	expires, err := time.Parse("2006-01-02", p.Expires)
	if err != nil || !expires.After(reviewed) || expires.Sub(reviewed) > 31*24*time.Hour || now.Before(reviewed) || !now.Before(expires) {
		return nil, nil
	}
	switch p.Basis {
	case "component_absent":
		if len(p.AbsentFiles) == 0 || len(p.AbsentArtifacts) == 0 {
			return nil, nil
		}
	case "unaffected_source", "unaffected_abi":
		if len(p.AbsentFiles) != 0 || len(p.AbsentArtifacts) != 0 {
			return nil, nil
		}
	default:
		return nil, nil
	}
	meta := sbom.Source.Metadata
	if sbom.Source.Type != "image" || meta.OS != "linux" || (meta.Architecture != "amd64" && meta.Architecture != "arm64") || meta.Labels["org.opencontainers.image.title"] != p.Image || !regexp.MustCompile(`^sha256:[a-f0-9]{64}$`).MatchString(meta.ManifestDigest) {
		return nil, nil
	}
	if sbom.Inventory == nil || sbom.Inventory.ManifestDigest != meta.ManifestDigest || len(sbom.Inventory.Files) == 0 {
		return nil, nil
	}
	config := sbom.Descriptor.Configuration
	if sbom.Descriptor.Name != "syft" || config.Files.Selection != "all" || config.Search.Scope != "squashed" {
		return nil, nil
	}
	catalogers := map[string]bool{}
	for _, name := range config.Catalogers.Used {
		catalogers[name] = true
	}
	if !catalogers["file-metadata-cataloger"] || !catalogers["file-digest-cataloger"] || len(sbom.Files) == 0 {
		return nil, nil
	}
	for _, pattern := range p.AbsentFiles {
		if len(pattern) < 3 || strings.ContainsAny(pattern, "/\\") || strings.Trim(pattern, "*?") == "" {
			return nil, nil
		}
		if _, err := path.Match(pattern, "probe"); err != nil {
			return nil, nil
		}
	}
	for name, file := range sbom.Inventory.Files {
		for _, pattern := range p.AbsentFiles {
			for _, candidate := range []string{name, file.Link} {
				if matched, _ := path.Match(strings.ToLower(pattern), strings.ToLower(path.Base(candidate))); matched {
					return nil, nil
				}
			}
		}
	}
	seen := map[string]bool{}
	for _, file := range sbom.Files {
		name := file.Location.Path
		if !strings.HasPrefix(name, "/") || path.Clean(name) != name || seen[name] || file.Metadata.Type == "" {
			return nil, nil
		}
		seen[name] = true
		for _, pattern := range p.AbsentFiles {
			for _, candidate := range []string{name, file.Metadata.LinkDestination} {
				if matched, _ := path.Match(strings.ToLower(pattern), strings.ToLower(path.Base(candidate))); matched {
					return nil, nil
				}
			}
		}
	}
	for _, absent := range p.AbsentArtifacts {
		if absent.Name == "" || absent.Type == "" {
			return nil, nil
		}
		for _, artifact := range sbom.Artifacts {
			if artifact.Name == absent.Name && artifact.Type == absent.Type {
				return nil, nil
			}
		}
	}
	variant := p.Variants[meta.Architecture]
	if len(variant.Packages) == 0 || len(variant.Files) == 0 {
		return nil, nil
	}
	fingerprint, err := sbom.Inventory.Fingerprint()
	if err != nil || !regexp.MustCompile(`^[a-f0-9]{64}$`).MatchString(variant.InventorySHA256) || fingerprint != variant.InventorySHA256 {
		return nil, nil
	}
	if (p.Basis == "component_absent") != (len(variant.AbsentSHA256) > 0) {
		return nil, nil
	}
	forbidden := map[string]bool{}
	for _, hash := range variant.AbsentSHA256 {
		if !regexp.MustCompile(`^[a-f0-9]{64}$`).MatchString(hash) || forbidden[hash] {
			return nil, nil
		}
		forbidden[hash] = true
	}
	for _, file := range sbom.Inventory.Files {
		if forbidden[file.SHA256] {
			return nil, nil
		}
	}
	for name, hash := range variant.Files {
		file, found := sbom.Inventory.Files[name]
		if !found || file.Kind != "RegularFile" || file.SHA256 != hash {
			return nil, nil
		}
	}
	for _, file := range sbom.Files {
		if _, anchored := variant.Files[file.Location.Path]; anchored && file.Metadata.Type != "RegularFile" {
			return nil, nil
		}
	}
	groups := map[string][]string{}
	for name, version := range variant.Packages {
		if name == "" || version == "" {
			return nil, nil
		}
		groups[version] = append(groups[version], name)
	}
	// Reuse the strict package identity, distro, architecture and SHA-256
	// validation; group by version to support native/all companion packages.
	versions := make([]string, 0, len(groups))
	for version := range groups {
		versions = append(versions, version)
	}
	sort.Strings(versions)
	rules := []Rule{}
	evidence := []map[string]any{}
	for _, version := range versions {
		packages := groups[version]
		sort.Strings(packages)
		r, e := backportReview(sbom, Backport{
			Namespace: p.Namespace, Codename: p.Codename, Version: version,
			Packages: packages, Advisories: p.Advisories, Sources: p.Sources, Reason: p.Reason,
			Variants: map[string]map[string]string{meta.Architecture: variant.Files, "all": variant.Files},
		})
		if len(r) == 0 || len(e) == 0 {
			return nil, nil
		}
		for _, entry := range e {
			entry["review_id"] = p.ID
			entry["basis"] = p.Basis
			entry["owner"] = p.Owner
			entry["reviewed"] = p.Reviewed
			entry["expires"] = p.Expires
			entry["image_manifest"] = meta.ManifestDigest
			entry["image_architecture"] = meta.Architecture
			entry["inventory_files"] = len(sbom.Files)
			entry["oci_inventory_files"] = len(sbom.Inventory.Files)
			entry["oci_inventory_sha256"] = fingerprint
			entry["absent_file_names"] = p.AbsentFiles
			entry["absent_artifacts"] = p.AbsentArtifacts
			entry["absent_sha256"] = variant.AbsentSHA256
		}
		rules = append(rules, r...)
		evidence = append(evidence, e...)
	}
	return rules, evidence
}
