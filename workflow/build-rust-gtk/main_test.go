package main

import (
	"github.com/tdesaules/github-actions/shared/rustgtk"
	"testing"
)

func TestBuildRejectsInvalidIdentityBeforeCommands(t *testing.T) {
	if build(rustgtk.Inputs{Version: "1.0.0;echo wrong"}) == nil {
		t.Fatal("accepted invalid identity")
	}
}
