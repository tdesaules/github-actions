package main

import (
	"github.com/tdesaules/github-actions/shared/rustgtk"
	"os"
	"path/filepath"
	"testing"
)

func TestQualificationRejectsUntrustedReceiptBeforeCommands(t *testing.T) {
	p := filepath.Join(t.TempDir(), "receipt.json")
	os.WriteFile(p, []byte(`{"schema_version":1,"tested":true}`), 0644)
	if qualify(rustgtk.Inputs{Receipt: p}) == nil {
		t.Fatal("accepted already qualified receipt")
	}
}
