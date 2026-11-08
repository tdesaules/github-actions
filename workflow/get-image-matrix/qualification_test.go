package main

import (
	"os"
	"strings"
	"testing"
)

const qualificationFixture = `name: main
jobs:
  build:
    runs-on: ubuntu-24.04
    steps:
      - uses: ./oci/build-image
  test:
    runs-on: ubuntu-24.04
    steps:
      - name: Test A
        if: matrix.image == 'oci-a'
        uses: ./.candidate-actions/oci/a
        with:
          directory: tests/fixtures/a
          expected: old-a
      - name: Test B
        if: matrix.image == 'oci-b'
        uses: ./.candidate-actions/oci/b
        with:
          directory: tests/fixtures/b
          expected: old-b
      - name: Test subset
        if: contains(fromJSON('["oci-a"]'), matrix.image)
        uses: ./workflow/helper
        with:
          expected: old-subset
      - name: Advisory scan
        id: security-sbom
        continue-on-error: true
        uses: ./workflow/helper
  maintenance:
    steps:
      - name: Maintenance old
        uses: ./maintenance/update-dependencies
`

func TestQualificationSelectionUsesWorkflowConsumers(t *testing.T) {
	for _, tc := range []struct {
		name, path, from, to string
		want                 []string
	}{
		{"maintenance only", ".github/workflows/main.yml", "Maintenance old", "Maintenance new", nil},
		{"invalid previous workflow", ".github/workflows/main.yml", "", "", []string{"oci-a", "oci-b"}},
		{"one image test", ".github/workflows/main.yml", "old-a", "new-a", []string{"oci-a"}},
		{"scoped helper test", ".github/workflows/main.yml", "old-subset", "new-subset", []string{"oci-a"}},
		{"native runner changed", ".github/workflows/main.yml", "ubuntu-24.04", "ubuntu-24.04-arm", []string{"oci-a", "oci-b"}},
		{"fixture file", "tests/fixtures/a/input.json", "", "", []string{"oci-a"}},
		{"deleted fixture", "tests/fixtures/b/deleted.json", "", "", []string{"oci-b"}},
		{"metadata", "oci/a/action.yml", "", "", []string{"oci-a"}},
		{"local tooling", "scripts/ci.py", "", "", nil},
		{"local task configuration", "taskfile.yml", "", "", nil},
		{"scoped helper source", "shared/helper/tool.go", "", "", []string{"oci-a"}},
		{"scoped helper metadata", "workflow/helper/action.yml", "", "", []string{"oci-a"}},
		{"deleted helper recipe", "workflow/helper/Containerfile", "", "", []string{"oci-a"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			git, write := sourceFixture(t)
			write(".github/workflows/main.yml", qualificationFixture)
			if tc.name == "invalid previous workflow" {
				write(".github/workflows/main.yml", "jobs: [\n")
			}
			write("workflow/helper/Containerfile", "FROM scratch AS final\nCOPY shared/helper/ /tool/\n")
			write("shared/helper/tool.go", "helper")
			git("add", ".")
			git("commit", "-m", "baseline")
			base := git("rev-parse", "HEAD")
			if tc.name == "invalid previous workflow" {
				write(".github/workflows/main.yml", qualificationFixture)
			}
			if tc.name == "deleted helper recipe" {
				if err := os.Remove("workflow/helper/Containerfile"); err != nil {
					t.Fatal(err)
				}
			}
			if tc.from != "" {
				write(tc.path, strings.ReplaceAll(qualificationFixture, tc.from, tc.to))
			}
			components := []component{{"oci/a", "oci-a", "example@sha256:" + strings.Repeat("a", 64)}, {"oci/b", "oci-b", "example@sha256:" + strings.Repeat("b", 64)}}
			m, err := selectImages(components, []string{tc.path}, false, rebuildHistory{before: base})
			if err != nil {
				t.Fatal(err)
			}
			if len(m.Include) != 2*len(tc.want) {
				t.Fatalf("got %+v, want images %v", m, tc.want)
			}
			for _, j := range m.Include {
				found := false
				for _, name := range tc.want {
					found = found || j.Image == name
				}
				if !found || j.Rebuild {
					t.Fatalf("test dependency should select without rebuilding: %+v", j)
				}
			}
		})
	}
}

func TestUnknownQualificationConditionIsConservative(t *testing.T) {
	before, err := readQualification([]byte(strings.Replace(qualificationFixture, "matrix.image == 'oci-a'", "matrix.image == 'oci-a' || inputs.extra", 1)))
	if err != nil {
		t.Fatal(err)
	}
	after, err := readQualification([]byte(strings.Replace(strings.Replace(qualificationFixture, "matrix.image == 'oci-a'", "matrix.image == 'oci-a' || inputs.extra", 1), "old-a", "new-a", 1)))
	if err != nil {
		t.Fatal(err)
	}
	if !workflowAffects(before, after, "oci-b") {
		t.Fatal("unknown condition silently omitted a possible consumer")
	}
}

func TestReleaseQualificationKeepsUnchangedImagesWithoutRebuilding(t *testing.T) {
	_, write := sourceFixture(t)
	write(".github/workflows/main.yml", qualificationFixture)
	write("workflow/helper/Containerfile", "FROM scratch AS final\n")
	components := []component{{"oci/a", "oci-a", "example@sha256:" + strings.Repeat("a", 64)}, {"oci/b", "oci-b", "example@sha256:" + strings.Repeat("b", 64)}}
	m, err := selectImages(components, nil, false, rebuildHistory{allTests: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Include) != 4 {
		t.Fatalf("release must retain both images: %+v", m)
	}
	for _, j := range m.Include {
		if j.Rebuild || !j.Scan {
			t.Fatalf("unchanged release image should be qualified/scanned, not rebuilt: %+v", j)
		}
	}
}
