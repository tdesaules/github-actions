package main

import "testing"

func TestTransferCannotRedirectToAnExternalRegistry(t *testing.T) {
	for _, repository := range []string{"example.org/image", "../outside", "image@sha256:abc", "image:tag", "image\nname"} {
		if err := execute([]string{"--archive=fixture.tar", "--repository=" + repository}); err == nil {
			t.Fatalf("accepted %q", repository)
		}
	}
	for _, port := range []string{"0", "65536", "external:5000"} {
		if err := execute([]string{"--archive=fixture.tar", "--repository=fixture", "--host-port=" + port}); err == nil {
			t.Fatalf("accepted port %q", port)
		}
	}
}
