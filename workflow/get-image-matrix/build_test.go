package main

import (
	"reflect"
	"testing"
)

func TestBuildMatrix(t *testing.T) {
	product := job{Image: "oci-new-sbom", Directory: "oci/new-sbom", Arch: "amd64", Runner: "ubuntu-24.04", Rebuild: true, Scan: true}
	published := job{Image: "oci-export-image", Arch: "amd64", Runner: "ubuntu-24.04"}
	reviewer := job{Image: "workflow-new-vulnerability-review", Directory: "workflow/new-vulnerability-review", Arch: "amd64", Runner: "ubuntu-24.04", Rebuild: true, Dependency: true}
	arm := published
	arm.Arch, arm.Runner, arm.Scan = "arm64", "ubuntu-24.04-arm", true
	selectedReviewer := reviewer
	selectedReviewer.Dependency = false
	scanOnly := published
	scanOnly.Scan = true
	for _, tc := range []struct {
		name        string
		input, want []job
	}{
		{"empty", nil, []job{}},
		{"published metadata only", []job{published}, []job{}},
		{"mixed candidates and published", []job{published, product}, []job{product}},
		{"scan only", []job{scanOnly, arm}, []job{}},
		{"no implicit helper build", []job{product, scanOnly}, []job{product}},
		{"reviewer is already a required product", []job{product, selectedReviewer}, []job{product, selectedReviewer}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			input := append([]job(nil), tc.input...)
			got := buildMatrix(matrix{Include: tc.input})
			if !reflect.DeepEqual(got.Include, tc.want) {
				t.Fatalf("got %+v; want %+v", got.Include, tc.want)
			}
			if !reflect.DeepEqual(input, tc.input) {
				t.Fatal("build filtering mutated the qualification matrix")
			}
		})
	}
}
