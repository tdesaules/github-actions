package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestVersionAndChecksumUpdate(t *testing.T) {
	oldSum := "h1:" + strings.Repeat("A", 43) + "="
	newSum := "h1:" + strings.Repeat("B", 43) + "="
	line := "RUN sh shared/runtime/compile-tool.sh github.com/example/tool v1.2.4 " + oldSum + " tool golang.org/x/net@v0.60.0\n"
	out, err := update(line, func(module, version string) (string, error) {
		if module != "github.com/example/tool" || version != "v1.2.4" {
			t.Fatalf("wrong source %s %s", module, version)
		}
		return newSum, nil
	})
	if err != nil || out != strings.Replace(line, oldSum, newSum, 1) {
		t.Fatalf("%q %v", out, err)
	}
	for _, bad := range []string{"deadbeef", "h1:bad", ""} {
		if _, err := update(line, func(string, string) (string, error) { return bad, nil }); err == nil {
			t.Fatal("invalid checksum accepted")
		}
	}
	if _, err := update(line, func(string, string) (string, error) { return "", errors.New("sumdb unavailable") }); err == nil {
		t.Fatal("lookup failure ignored")
	}
}

func TestDownloadRejectsWrongModuleAndForcesVerification(t *testing.T) {
	t.Setenv("GOSUMDB", "off")
	t.Setenv("GOPRIVATE", "*")
	dir := t.TempDir()
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	for _, module := range []string{"github.com/example/tool", "github.com/other/tool"} {
		script := "#!/bin/sh\n[ \"$GOSUMDB\" = sum.golang.org ] && [ -z \"$GOPRIVATE\" ] && [ \"$GOWORK\" = off ] || exit 9\n" +
			"[ \"$1 $2 $3 $4\" = 'mod download -json github.com/example/tool@v1.2.4' ] || exit 8\n" +
			"printf '%s' '{\"Path\":\"" + module + "\",\"Version\":\"v1.2.4\",\"Sum\":\"h1:" + strings.Repeat("A", 43) + "=\"}'\n"
		if err := os.WriteFile(filepath.Join(dir, "go"), []byte(script), 0755); err != nil {
			t.Fatal(err)
		}
		_, err := download("github.com/example/tool", "v1.2.4")
		if (err != nil) != (module != "github.com/example/tool") {
			t.Fatalf("%s: %v", module, err)
		}
	}
}

func TestUnrelatedPinsRemainUntouched(t *testing.T) {
	line := "go mod edit -require=github.com/example/tool@v1.2.4\n"
	out, err := update(line, func(string, string) (string, error) { t.Fatal("unexpected lookup"); return "", nil })
	if err != nil || out != line {
		t.Fatal("unrelated dependency modified")
	}
}
