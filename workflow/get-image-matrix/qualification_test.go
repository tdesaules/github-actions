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
				// This fixture copies the entire action directory: metadata is a
				// real build input here, unlike an entrypoint-only recipe.
				if !found || j.Rebuild != (tc.name == "metadata") {
					t.Fatalf("unexpected test/build selection: %+v", j)
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

func TestNativeHarnessConditionsKeepImageScope(t *testing.T) {
	for _, tc := range []struct {
		condition string
		want      []string
	}{
		{"matrix.image == 'oci-a' || (matrix.image == 'oci-b' && matrix.arch == 'amd64')", []string{"oci-a", "oci-b"}},
		{`${{ contains(fromJSON('["oci-a"]'), matrix.image) && matrix.arch == 'arm64' }}`, []string{"oci-a"}},
		{"matrix.image == 'oci-a' && matrix.arch == 'amd64'", []string{"oci-a"}},
		{"matrix.image == 'oci-a' || inputs.extra", []string{"oci-a", "oci-b", "check-verifier"}},
		{"matrix.image == 'oci-a' && matrix.arch == 'unknown-future-platform'", []string{"oci-a", "oci-b", "check-verifier"}},
	} {
		t.Run(tc.condition, func(t *testing.T) {
			for _, image := range []string{"oci-a", "oci-b", "check-verifier"} {
				want := false
				for _, selected := range tc.want {
					want = want || image == selected
				}
				if got := appliesToImage(map[string]any{"if": tc.condition}, image); got != want {
					t.Fatalf("%s: scope=%t, want %t", image, got, want)
				}
			}
		})
	}
}

const harnessQualificationFixture = `name: image
jobs:
  test:
    steps:
      - name: Build repository-only synthetic helper
        if: contains(fromJSON('["oci-a"]'), matrix.image)
        uses: ./oci/build-image
        with:
          context: .
          containerfile: .github/scripts/atomic-fixtures/Containerfile
      - name: Build action verifier harness
        if: matrix.image == 'oci-b' && matrix.arch == 'amd64'
        uses: ./oci/build-image
        with:
          context: .
          containerfile: check/verifier/Containerfile
      - name: Run generated helper
        if: contains(fromJSON('["oci-a"]'), matrix.image)
        uses: ./.candidate-actions/.github/scripts/atomic-fixtures
      - name: Run generated verifier
        if: matrix.image == 'oci-b' && matrix.arch == 'amd64'
        uses: ./.candidate-actions/check/verifier
`

func TestHarnessSourcesRequalifyConsumersWithoutRebuildingThem(t *testing.T) {
	for _, tc := range []struct {
		name, path string
		delete     bool
		want       map[string]bool // selected image -> rebuild; absent means no work
	}{
		{"hidden helper executable", ".github/scripts/atomic-fixtures/main.go", false, map[string]bool{"oci-a": false}},
		{"hidden helper tests", ".github/scripts/atomic-fixtures/main_test.go", false, map[string]bool{"oci-a": false}},
		{"hidden helper metadata", ".github/scripts/atomic-fixtures/action.yml", false, map[string]bool{"oci-a": false}},
		{"hidden helper recipe", ".github/scripts/atomic-fixtures/Containerfile", false, map[string]bool{"oci-a": false}},
		{"deleted hidden helper recipe", ".github/scripts/atomic-fixtures/Containerfile", true, map[string]bool{"oci-a": false}},
		{"deleted hidden helper source", ".github/scripts/atomic-fixtures/main.go", true, map[string]bool{"oci-a": false}},
		{"copied shared helper source", "shared/fixture-helper/tool.go", false, map[string]bool{"oci-a": false}},
		{"ignored shared helper source", "shared/fixture-helper/ignored.go", false, nil},
		{"helper docs only", ".github/scripts/atomic-fixtures/README.md", false, nil},
		{"unrelated hidden source", ".github/scripts/unrelated/main.go", false, nil},
		{"action harness executable", "check/verifier/main.go", false, map[string]bool{"oci-b": false, "check-verifier": true}},
		{"action harness metadata", "check/verifier/action.yml", false, map[string]bool{"oci-b": false, "check-verifier": false}},
		{"action harness copied shared source", "shared/verifier/tool.go", false, map[string]bool{"oci-b": false, "check-verifier": true}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			git, write := sourceFixture(t)
			write(".github/workflows/image.yml", harnessQualificationFixture)
			write(".containerignore", "shared/fixture-helper/ignored.go\n")
			write(".github/scripts/atomic-fixtures/Containerfile", "FROM scratch AS final\nCOPY .github/scripts/atomic-fixtures/main.go /fixture\nCOPY shared/fixture-helper/ /helper/\n")
			write(".github/scripts/atomic-fixtures/main.go", "fixture\n")
			write("shared/fixture-helper/tool.go", "helper\n")
			write("check/verifier/Containerfile", "FROM scratch AS final\nCOPY check/verifier/main.go /verifier\nCOPY shared/verifier/ /shared/\n")
			write("check/verifier/main.go", "verifier\n")
			write("shared/verifier/tool.go", "verifier library\n")
			git("add", ".")
			git("commit", "-m", "fixture baseline")
			base := git("rev-parse", "HEAD")
			if tc.delete {
				if err := os.Remove(tc.path); err != nil {
					t.Fatal(err)
				}
			}
			components := []component{
				{"oci/a", "oci-a", "example@sha256:" + strings.Repeat("a", 64)},
				{"oci/b", "oci-b", "example@sha256:" + strings.Repeat("b", 64)},
				{"check/verifier", "check-verifier", "example@sha256:" + strings.Repeat("c", 64)},
			}
			m, err := selectImages(components, []string{tc.path}, false, rebuildHistory{before: base})
			if err != nil {
				t.Fatal(err)
			}
			if len(m.Include) != 2*len(tc.want) {
				t.Fatalf("got %+v, expected only %v", m.Include, tc.want)
			}
			for _, entry := range m.Include {
				wantRebuild, wanted := tc.want[entry.Image]
				if !wanted || entry.Rebuild != wantRebuild || !entry.RuntimeTest {
					t.Fatalf("unexpected consumer/build selection: %+v", entry)
				}
			}
		})
	}
}
