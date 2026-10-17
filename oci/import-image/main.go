package main

import action "github.com/tdesaules/github-actions/shared/runtime"

func main() {
	action.Main("import-image", "skopeo", []string{"source", "destination"})
}
