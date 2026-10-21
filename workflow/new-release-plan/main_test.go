package main

import "testing"

func TestStableTags(t *testing.T) {
	if got := latest("1.9.0\n1.10.0\n1.11.0-rc.1\nv2.0.0\n01.0.0\n1.10.0+build"); got != "1.10.0" {
		t.Fatal(got)
	}
	if latest("0.0.0-bootstrap") == "0.0.0-bootstrap" {
		t.Fatal("prerelease selected")
	}
}
