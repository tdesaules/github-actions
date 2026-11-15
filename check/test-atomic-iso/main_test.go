package main

import (
	"strings"
	"testing"
)

func TestSourceBinding(t *testing.T) {
	source, archive := strings.Repeat("a", 40), strings.Repeat("b", 64)
	r := receipt{
		SchemaVersion: 1, Component: "iso", Arch: "amd64", SourceSHA: source,
		Version: "1.0.0", SHA256: strings.Repeat("c", 64), Size: 1024,
		OSArchiveSHA256: archive,
	}
	if err := validate(r, source, "1.0.0", archive); err != nil {
		t.Fatal(err)
	}
	if err := validate(r, source, "1.0.1", archive); err == nil {
		t.Fatal("wrong release accepted")
	}
	if err := validate(r, strings.Repeat("d", 40), "1.0.0", archive); err == nil {
		t.Fatal("wrong source accepted")
	}
	if err := validate(r, source, "1.0.0", strings.Repeat("e", 64)); err == nil {
		t.Fatal("different OS archive accepted")
	}
	r.Tested = true
	if err := validate(r, source, "1.0.0", archive); err == nil {
		t.Fatal("qualified receipt accepted as an untested build receipt")
	}
}
