package main

import "testing"

func TestFedoraMajorVersionException(t *testing.T) {
	for reference, want := range map[string]bool{
		"quay.io/fedora/fedora:45":         true,
		"quay.io/fedora/fedora:44":         true,
		"quay.io/fedora/fedora-minimal:45": true,
		"quay.io/fedora/fedora:latest":     false,
		"quay.io/fedora/fedora:rawhide":    false,
		"quay.io/fedora/fedora:45.1":       false,
		"quay.io/fedora/fedora":            false,
		"quay.io/other/fedora:45":          false,
		"docker.io/library/alpine:3.24":    false,
	} {
		if got := versionedBase(reference); got != want {
			t.Errorf("%s: got %t, want %t", reference, got, want)
		}
	}
}
