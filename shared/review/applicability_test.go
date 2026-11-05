package review

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"testing"
	"time"
)

func applicabilityFixture(t *testing.T) (SBOM, Applicability, time.Time) {
	t.Helper()
	hash := strings.Repeat("a", 64)
	var sbom SBOM
	err := json.Unmarshal([]byte(fmt.Sprintf(`{
 "distro":{"id":"debian","versionCodename":"trixie"},
 "source":{"type":"image","metadata":{"architecture":"amd64","os":"linux","manifestDigest":"sha256:%s","labels":{"org.opencontainers.image.title":"workflow-build-pantavisor"}}},
 "descriptor":{"name":"syft","configuration":{"files":{"selection":"all"},"catalogers":{"used":["file-metadata-cataloger","file-digest-cataloger"]},"search":{"scope":"squashed"}}},
 "artifacts":[{"id":"native","name":"libexample","version":"1.0-1","type":"deb","metadata":{"architecture":"amd64"}}],
 "files":[{"location":{"path":"/usr/lib/libexample.so"},"metadata":{"type":"RegularFile"},"digests":[{"algorithm":"sha256","value":"%s"}]}]
}`, hash, hash)), &sbom)
	if err != nil {
		t.Fatal(err)
	}
	sbom.Inventory = &Inventory{ManifestDigest: sbom.Source.Metadata.ManifestDigest, Files: map[string]inventoryFile{
		"/usr/lib/libexample.so": {Kind: "RegularFile", SHA256: hash},
	}}
	fingerprint, err := sbom.Inventory.Fingerprint()
	if err != nil {
		t.Fatal(err)
	}
	p := Applicability{
		ID: "missing-tool", Image: "workflow-build-pantavisor", Namespace: "debian:distro:debian:13", Codename: "trixie",
		Basis: "component_absent", Owner: "catalogue", Reviewed: "2026-11-05", Expires: "2026-12-05", Reason: "Affected command is absent",
		Advisories:  map[string]string{"CVE-2026-12345": "https://example.org/advisory"},
		Sources:     map[string]string{"https://example.org/source.tar.xz": hash},
		AbsentFiles: []string{"missing-tool*"}, AbsentArtifacts: []AbsentArtifact{{Name: "missing-tool", Type: "deb"}},
		Variants: map[string]ApplicabilityVariant{"amd64": {
			InventorySHA256: fingerprint,
			Packages:        map[string]string{"libexample": "1.0-1"}, Files: map[string]string{"/usr/lib/libexample.so": hash},
			AbsentSHA256: []string{strings.Repeat("f", 64)},
		}},
	}
	return sbom, p, time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC)
}

func TestApplicabilityRejectsUnprovenAbsence(t *testing.T) {
	cases := map[string]func(*SBOM, *Applicability){
		"unexpected new payload": func(s *SBOM, _ *Applicability) {
			s.Inventory.Files["/opt/unreviewed"] = inventoryFile{Kind: "RegularFile", SHA256: strings.Repeat("d", 64)}
		},
		"changed mode": func(s *SBOM, _ *Applicability) {
			f := s.Inventory.Files["/usr/lib/libexample.so"]
			f.Mode = 04755
			s.Inventory.Files["/usr/lib/libexample.so"] = f
		},
		"renamed forbidden component": func(s *SBOM, _ *Applicability) {
			s.Inventory.Files["/opt/renamed"] = inventoryFile{Kind: "RegularFile", SHA256: strings.Repeat("f", 64)}
		},
		"no OCI inventory":    func(s *SBOM, _ *Applicability) { s.Inventory = nil },
		"wrong OCI inventory": func(s *SBOM, _ *Applicability) { s.Inventory.ManifestDigest = "sha256:" + strings.Repeat("c", 64) },
		"filtered SBOM hides component": func(s *SBOM, _ *Applicability) {
			s.Inventory.Files["/usr/bin/missing-tool"] = inventoryFile{Kind: "RegularFile"}
		},
		"filtered SBOM hides alias": func(s *SBOM, _ *Applicability) {
			s.Inventory.Files["/usr/bin/alias"] = inventoryFile{Kind: "SymbolicLink", Link: "missing-tool"}
		},
		"archive anchor altered": func(s *SBOM, _ *Applicability) {
			s.Inventory.Files["/usr/lib/libexample.so"] = inventoryFile{Kind: "RegularFile", SHA256: strings.Repeat("b", 64)}
		},
		"expired":                  func(_ *SBOM, p *Applicability) { p.Reviewed = "2026-11-04"; p.Expires = "2026-11-05" },
		"future review":            func(_ *SBOM, p *Applicability) { p.Reviewed = "2026-11-06" },
		"unbounded window":         func(_ *SBOM, p *Applicability) { p.Expires = "2027-01-01" },
		"bad date":                 func(_ *SBOM, p *Applicability) { p.Expires = "never" },
		"missing owner":            func(_ *SBOM, p *Applicability) { p.Owner = "" },
		"risk acceptance":          func(_ *SBOM, p *Applicability) { p.Basis = "risk_accepted" },
		"no absence paths":         func(_ *SBOM, p *Applicability) { p.AbsentFiles = nil },
		"no absence packages":      func(_ *SBOM, p *Applicability) { p.AbsentArtifacts = nil },
		"malformed pattern":        func(_ *SBOM, p *Applicability) { p.AbsentFiles = []string{"tool["} },
		"missing all-files marker": func(s *SBOM, _ *Applicability) { s.Descriptor.Configuration.Files.Selection = "owned-by-package" },
		"missing cataloger": func(s *SBOM, _ *Applicability) {
			s.Descriptor.Configuration.Catalogers.Used = []string{"file-digest-cataloger"}
		},
		"wrong scope":      func(s *SBOM, _ *Applicability) { s.Descriptor.Configuration.Search.Scope = "all-layers" },
		"wrong generator":  func(s *SBOM, _ *Applicability) { s.Descriptor.Name = "unknown" },
		"not an image":     func(s *SBOM, _ *Applicability) { s.Source.Type = "directory" },
		"missing identity": func(s *SBOM, _ *Applicability) { s.Source.Metadata.ManifestDigest = "" },
		"wrong owner image": func(s *SBOM, _ *Applicability) {
			s.Source.Metadata.Labels["org.opencontainers.image.title"] = "another-action"
		},
		"wrong architecture":         func(s *SBOM, _ *Applicability) { s.Source.Metadata.Architecture = "arm64" },
		"wrong package architecture": func(s *SBOM, _ *Applicability) { s.Artifacts[0].Metadata.Architecture = "arm64" },
		"wrong distro":               func(s *SBOM, _ *Applicability) { s.Distro.Codename = "bookworm" },
		"missing package":            func(s *SBOM, _ *Applicability) { s.Artifacts = nil },
		"wrong version":              func(s *SBOM, _ *Applicability) { s.Artifacts[0].Version = "1.0-2" },
		"duplicate package":          func(s *SBOM, _ *Applicability) { s.Artifacts = append(s.Artifacts, s.Artifacts[0]) },
		"missing payload":            func(s *SBOM, _ *Applicability) { s.Files = nil },
		"altered payload":            func(s *SBOM, _ *Applicability) { s.Files[0].Digests[0].Value = strings.Repeat("b", 64) },
		"duplicate path":             func(s *SBOM, _ *Applicability) { s.Files = append(s.Files, s.Files[0]) },
		"noncanonical path":          func(s *SBOM, _ *Applicability) { s.Files[0].Location.Path = "/usr/lib/../lib/libexample.so" },
		"incomplete metadata":        func(s *SBOM, _ *Applicability) { s.Files[0].Metadata.Type = "" },
		"anchor replaced by alias":   func(s *SBOM, _ *Applicability) { s.Files[0].Metadata.Type = "SymbolicLink" },
		"component present": func(s *SBOM, _ *Applicability) {
			f := s.Files[0]
			f.Location.Path = "/opt/missing-tool"
			s.Files = append(s.Files, f)
		},
		"component alias": func(s *SBOM, _ *Applicability) {
			f := s.Files[0]
			f.Location.Path = "/opt/alias"
			f.Metadata.Type = "SymbolicLink"
			f.Metadata.LinkDestination = "../bin/missing-tool"
			s.Files = append(s.Files, f)
		},
		"component package present": func(s *SBOM, _ *Applicability) {
			a := s.Artifacts[0]
			a.ID = "added"
			a.Name = "missing-tool"
			s.Artifacts = append(s.Artifacts, a)
		},
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			s, p, now := applicabilityFixture(t)
			mutate(&s, &p)
			rules, evidence := applicabilityReview(s, p, now)
			if len(rules) != 0 || len(evidence) != 0 {
				t.Fatalf("unproven applicability emitted rules: %+v", rules)
			}
		})
	}
}

func TestApplicabilityRequiresEveryCompanionAndRecordsExpiry(t *testing.T) {
	s, p, now := applicabilityFixture(t)
	a := s.Artifacts[0]
	a.ID, a.Name, a.Version, a.Metadata.Architecture = "companion", "example-data", "1.0-1+data", "all"
	s.Artifacts = append(s.Artifacts, a)
	p.Variants["amd64"].Packages[a.Name] = a.Version
	rules, evidence := applicabilityReview(s, p, now)
	if len(rules) != 2 || len(evidence) != 2 || evidence[0]["expires"] != p.Expires || evidence[0]["image_manifest"] != s.Source.Metadata.ManifestDigest {
		t.Fatalf("missing scoped rules/evidence: %+v %+v", rules, evidence)
	}
	s.Artifacts[1].Version = "unexpected"
	rules, _ = applicabilityReview(s, p, now)
	if len(rules) != 0 {
		t.Fatal("partial companion proof was accepted")
	}
}

func TestApplicabilityPositiveSourceAndABIProofs(t *testing.T) {
	for _, basis := range []string{"unaffected_source", "unaffected_abi"} {
		s, p, now := applicabilityFixture(t)
		p.Basis, p.AbsentFiles, p.AbsentArtifacts = basis, nil, nil
		variant := p.Variants["amd64"]
		variant.AbsentSHA256 = nil
		p.Variants["amd64"] = variant
		rules, _ := applicabilityReview(s, p, now)
		if len(rules) != 1 {
			t.Fatalf("valid %s evidence rejected", basis)
		}
		s.Files[0].Digests[0].Value = strings.Repeat("c", 64)
		rules, _ = applicabilityReview(s, p, now)
		if len(rules) != 0 {
			t.Fatalf("altered %s bytes accepted", basis)
		}
	}
}

func TestApplicabilityDuplicateIDsFailClosed(t *testing.T) {
	s, p, now := applicabilityFixture(t)
	config, _ := evaluateAt(s, Policy{Applicability: []Applicability{p, p}}, now)
	if len(config.Ignore) != 0 {
		t.Fatal("duplicate review identity accepted")
	}
}

func TestApplicabilityEscapesGrypePackageRegex(t *testing.T) {
	s, p, now := applicabilityFixture(t)
	s.Artifacts[0].Name = "libstdc++6"
	delete(p.Variants["amd64"].Packages, "libexample")
	p.Variants["amd64"].Packages["libstdc++6"] = "1.0-1"
	rules, _ := applicabilityReview(s, p, now)
	if len(rules) != 1 {
		t.Fatal("valid C++ package rejected")
	}
	pattern, err := regexp.Compile("^" + rules[0].Package["name"] + "$")
	if err != nil || !pattern.MatchString("libstdc++6") || pattern.MatchString("libstdccc6") || pattern.MatchString("libstdc++6-extra") {
		t.Fatal("Grype package pattern does not preserve exact identity")
	}
}

func TestApplicabilityExpiresAtUTCBoundary(t *testing.T) {
	s, p, _ := applicabilityFixture(t)
	deadline, err := time.Parse("2006-01-02", p.Expires)
	if err != nil {
		t.Fatal(err)
	}
	before, _ := applicabilityReview(s, p, deadline.Add(-time.Nanosecond))
	at, _ := applicabilityReview(s, p, deadline)
	if len(before) != 1 || len(at) != 0 {
		t.Fatal("expiry must be exclusive at UTC midnight")
	}
}
