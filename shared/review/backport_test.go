package review

import (
	"encoding/json"
	"strings"
	"testing"
)

func backportFixture(t *testing.T) (SBOM, Backport) {
	t.Helper()
	sum := strings.Repeat("a", 64)
	var sbom SBOM
	data := `{"distro":{"id":"debian","versionCodename":"trixie"},"artifacts":[{"id":"binary","name":"curl","version":"8.23.0-1~pantavisor1","type":"deb","metadata":{"architecture":"amd64"}},{"id":"library","name":"libcurl4t64","version":"8.23.0-1~pantavisor1","type":"deb","metadata":{"architecture":"amd64"}}],"files":[{"location":{"path":"/usr/bin/curl"},"digests":[{"algorithm":"sha256","value":"` + sum + `"}]}]}`
	if err := json.Unmarshal([]byte(data), &sbom); err != nil {
		t.Fatal(err)
	}
	p := Backport{Namespace: "debian:distro:debian:13", Codename: "trixie", Version: "8.23.0-1~pantavisor1", Packages: []string{"curl", "libcurl4t64"}, Advisories: map[string]string{"CVE-2026-10536": "https://curl.se/docs/CVE-2026-10536.html"}, Sources: map[string]string{"https://example.invalid/source.tar.gz": sum}, Variants: map[string]map[string]string{"amd64": {"/usr/bin/curl": sum}}, Reason: "Verified fixed upstream source and native payload"}
	return sbom, p
}

func TestBackportRequiresExactEvidence(t *testing.T) {
	sbom, p := backportFixture(t)
	rules, evidence := backportReview(sbom, p)
	if len(rules) != 2 || len(evidence) != 1 {
		t.Fatal("verified backport was not recognized")
	}
	for _, rule := range rules {
		if rule.Vulnerability != "CVE-2026-10536" || rule.Namespace != "debian:distro:debian:13" || rule.Package["version"] != p.Version || rule.Package["type"] != "deb" {
			t.Fatal("unbounded rule", rule)
		}
	}
	for name, mutate := range map[string]func(*SBOM, *Backport){
		"modified binary":  func(s *SBOM, _ *Backport) { s.Files[0].Digests[0].Value = strings.Repeat("b", 64) },
		"missing digest":   func(s *SBOM, _ *Backport) { s.Files[0].Digests = nil },
		"ambiguous file":   func(s *SBOM, _ *Backport) { s.Files = append(s.Files, s.Files[0]) },
		"ambiguous digest": func(s *SBOM, _ *Backport) { s.Files[0].Digests = append(s.Files[0].Digests, s.Files[0].Digests[0]) },
		"old version":      func(s *SBOM, _ *Backport) { s.Artifacts[0].Version = "8.14.1" },
		"extra old copy": func(s *SBOM, _ *Backport) {
			a := s.Artifacts[0]
			a.Version = "8.14.1"
			s.Artifacts = append(s.Artifacts, a)
		},
		"missing package":    func(s *SBOM, _ *Backport) { s.Artifacts = s.Artifacts[:1] },
		"wrong architecture": func(s *SBOM, _ *Backport) { s.Artifacts[0].Metadata.Architecture = "arm64" },
		"wrong distro":       func(s *SBOM, _ *Backport) { s.Distro.ID = "ubuntu" },
		"wrong release":      func(s *SBOM, _ *Backport) { s.Distro.Codename = "bookworm" },
		"wrong type":         func(s *SBOM, _ *Backport) { s.Artifacts[0].Type = "binary" },
		"empty files":        func(_ *SBOM, p *Backport) { p.Variants["amd64"] = nil },
		"invalid hash":       func(_ *SBOM, p *Backport) { p.Variants["amd64"]["/usr/bin/curl"] = "invalid" },
		"missing sources":    func(_ *SBOM, p *Backport) { p.Sources = nil },
		"wildcard advisory":  func(_ *SBOM, p *Backport) { p.Advisories = map[string]string{"*": "https://example.invalid"} },
	} {
		t.Run(name, func(t *testing.T) {
			s, p := backportFixture(t)
			mutate(&s, &p)
			rules, evidence := backportReview(s, p)
			if len(rules) != 0 || len(evidence) != 0 {
				t.Fatal("unproven backport accepted")
			}
		})
	}
}

func TestBackportArchitectureIndependentCompanion(t *testing.T) {
	sbom, p := backportFixture(t)
	companion := sbom.Artifacts[0]
	companion.ID, companion.Name, companion.Metadata.Architecture = "translations", "curl-l10n", "all"
	sbom.Artifacts = append([]Artifact{companion}, sbom.Artifacts...)
	p.Packages = append(p.Packages, companion.Name)
	if rules, _ := backportReview(sbom, p); len(rules) != 3 {
		t.Fatal("architecture-independent companion rejected")
	}
	sbom.Artifacts[0].Version = "old"
	if rules, _ := backportReview(sbom, p); len(rules) != 0 {
		t.Fatal("stale architecture-independent companion accepted")
	}
}

func TestBackportPureArchitectureIndependentPayload(t *testing.T) {
	sbom, p := backportFixture(t)
	for i := range sbom.Artifacts {
		sbom.Artifacts[i].Metadata.Architecture = "all"
	}
	if rules, _ := backportReview(sbom, p); len(rules) != 0 {
		t.Fatal("native hashes implicitly accepted for architecture-independent packages")
	}
	p.Variants["all"] = p.Variants["amd64"]
	if rules, _ := backportReview(sbom, p); len(rules) != 2 {
		t.Fatal("explicit architecture-independent proof rejected")
	}
}
