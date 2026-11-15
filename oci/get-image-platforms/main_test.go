package main

import (
	"fmt"
	"strings"
	"testing"
)

func TestPlatformSet(t *testing.T) {
	a, b := "sha256:"+strings.Repeat("a", 64), "sha256:"+strings.Repeat("b", 64)
	descriptor := func(arch, os, digest string) string {
		return fmt.Sprintf(`{"digest":%q,"platform":{"architecture":%q,"os":%q}}`, digest, arch, os)
	}
	prefix := `{"schemaVersion":2,"mediaType":"application/vnd.oci.image.index.v1+json","manifests":[`
	valid := prefix + descriptor("amd64", "linux", a) + `,` + descriptor("arm64", "linux", b) + `,` + descriptor("unknown", "unknown", a) + `]}`
	got, err := platforms([]byte(valid))
	if err != nil || got["amd64"] != a || got["arm64"] != b {
		t.Fatalf("valid index failed: %v %v", got, err)
	}
	for _, raw := range []string{
		`{}`,
		prefix + descriptor("amd64", "linux", a) + `]}`,
		prefix + descriptor("amd64", "linux", a) + `,` + descriptor("amd64", "linux", b) + `]}`,
		prefix + descriptor("amd64", "linux", "invalid") + `,` + descriptor("arm64", "linux", b) + `]}`,
	} {
		if _, err := platforms([]byte(raw)); err == nil {
			t.Fatalf("invalid index accepted: %s", raw)
		}
	}
}
