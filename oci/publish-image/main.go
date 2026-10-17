package main

import action "github.com/tdesaules/github-actions/shared/runtime"

func main() {
	action.Main("publish-image", "skopeo", []string{"source", "destination", "registry", "registry-username", "registry-password"})
}
