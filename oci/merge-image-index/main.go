package main

import action "github.com/tdesaules/github-actions/shared/runtime"

func main() {
	action.Main("merge-image-index", "skopeo", []string{"sources", "destination", "output", "registry", "registry-username", "registry-password"})
}
