package main

import action "github.com/tdesaules/github-actions/shared/runtime"

func main() {
	action.Main("test-image-signature", "cosign", []string{"image", "identity", "issuer", "key", "transparency-log", "output", "registry", "registry-username", "registry-password"})
}
