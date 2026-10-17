package main

import action "github.com/tdesaules/github-actions/shared/runtime"

func main() {
	action.Main("get-image-digest", "skopeo", []string{"image", "expected-digest", "registry", "registry-username", "registry-password"})
}
