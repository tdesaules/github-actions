package main

import action "github.com/tdesaules/github-actions/shared/runtime"

func main() {
	action.Main("new-image-signature", "cosign", []string{"image", "enabled", "key", "transparency-log", "registry", "registry-username", "registry-password"})
}
