package main

import action "github.com/tdesaules/github-actions/shared/runtime"

func main() {
	action.Main("export-image", "skopeo", []string{"source", "destination"})
}
