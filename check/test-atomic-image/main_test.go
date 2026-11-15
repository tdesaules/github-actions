package main

import (
	"strings"
	"testing"
)

func TestConfigIdentity(t *testing.T) {
	revision := strings.Repeat("a", 40)
	valid := []byte(`{"architecture":"amd64","os":"linux","config":{"Labels":{"org.opencontainers.image.revision":"` + revision + `"}}}`)
	if err := verifyConfig(valid, "amd64", revision); err != nil {
		t.Fatal(err)
	}
	if err := verifyConfig(valid, "arm64", revision); err == nil {
		t.Fatal("wrong architecture accepted")
	}
	if err := verifyConfig(valid, "amd64", strings.Repeat("b", 40)); err == nil {
		t.Fatal("wrong source accepted")
	}
	if err := verifyConfig([]byte(`{}`), "amd64", revision); err == nil {
		t.Fatal("missing metadata accepted")
	}
	if err := verifyConfig([]byte(`invalid`), "amd64", revision); err == nil {
		t.Fatal("invalid JSON accepted")
	}
}
