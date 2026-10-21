// Package review applies advisory exclusions only when SBOM evidence proves them.
package review

import (
	"path"
	"sort"
	"strings"
)

type Location struct {
	Path string `json:"path"`
}
type Artifact struct {
	ID        string     `json:"id"`
	Name      string     `json:"name"`
	Version   string     `json:"version"`
	Type      string     `json:"type"`
	Locations []Location `json:"locations"`
	Metadata  struct {
		Symbols map[string][]string `json:"symbols"`
	} `json:"metadata"`
}
type File struct {
	Location Location                            `json:"location"`
	Digests  []struct{ Algorithm, Value string } `json:"digests"`
}
type SBOM struct {
	Artifacts []Artifact `json:"artifacts"`
	Files     []File     `json:"files"`
}
type Policy struct {
	Vulnerability string `json:"vulnerability"`
	Package       string `json:"package"`
	Affected      string `json:"affected_package"`
	Reference     string `json:"reference"`
	Reason        string `json:"reason"`
	Patched       *Patch `json:"patched_npm"`
}
type Patch struct {
	Vulnerability string            `json:"vulnerability"`
	Package       string            `json:"package"`
	Version       string            `json:"version"`
	Reference     string            `json:"reference"`
	Reason        string            `json:"reason"`
	Files         map[string]string `json:"files"`
}
type Rule struct {
	Vulnerability string            `json:"vulnerability"`
	Namespace     string            `json:"namespace"`
	Package       map[string]string `json:"package"`
	Reason        string            `json:"reason"`
}
type Configuration struct {
	Ignore []Rule `json:"ignore"`
}

func Evaluate(sbom SBOM, policy Policy) (Configuration, []map[string]any) {
	config := Configuration{Ignore: []Rule{}}
	evidence := []map[string]any{}
	for _, a := range sbom.Artifacts {
		if a.Name != policy.Package {
			continue
		}
		nonempty := false
		packages := []string{}
		for name, symbols := range a.Metadata.Symbols {
			if name == policy.Affected || strings.HasPrefix(name, policy.Affected+"/") {
				return Configuration{Ignore: []Rule{}}, []map[string]any{}
			}
			nonempty = nonempty || len(symbols) > 0
			packages = append(packages, name)
		}
		if !nonempty {
			return Configuration{Ignore: []Rule{}}, []map[string]any{}
		}
		sort.Strings(packages)
		config.Ignore = append(config.Ignore, Rule{policy.Vulnerability, "govulndb:language:go", map[string]string{"name": a.Name, "version": a.Version, "type": "go-module"}, policy.Reason})
		evidence = append(evidence, map[string]any{"artifact_id": a.ID, "version": a.Version, "linked_packages": packages, "reference": policy.Reference})
	}
	rules, patched := patchedReview(sbom, policy.Patched)
	config.Ignore = append(config.Ignore, rules...)
	evidence = append(evidence, patched...)
	return config, evidence
}

func patchedReview(sbom SBOM, p *Patch) ([]Rule, []map[string]any) {
	if p == nil || len(p.Files) == 0 {
		return nil, nil
	}
	files := map[string][]File{}
	for _, f := range sbom.Files {
		files[f.Location.Path] = append(files[f.Location.Path], f)
	}
	evidence := []map[string]any{}
	for _, a := range sbom.Artifacts {
		if a.Name != p.Package || a.Type != "npm" {
			continue
		}
		if a.Version != p.Version || len(a.Locations) == 0 {
			return nil, nil
		}
		for _, location := range a.Locations {
			if path.Base(location.Path) != "package.json" {
				return nil, nil
			}
			checked := map[string]string{}
			for relative, expected := range p.Files {
				name := path.Join(path.Dir(location.Path), relative)
				matches := files[name]
				if len(matches) != 1 {
					return nil, nil
				}
				digests := []string{}
				for _, d := range matches[0].Digests {
					if d.Algorithm == "sha256" {
						digests = append(digests, d.Value)
					}
				}
				if len(digests) != 1 || digests[0] != expected {
					return nil, nil
				}
				checked[name] = expected
			}
			evidence = append(evidence, map[string]any{"artifact_id": a.ID, "version": a.Version, "patched_files": checked, "reference": p.Reference})
		}
	}
	if len(evidence) == 0 {
		return nil, nil
	}
	return []Rule{{p.Vulnerability, "github:language:javascript", map[string]string{"name": p.Package, "version": p.Version, "type": "npm"}, p.Reason}}, evidence
}
