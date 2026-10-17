package main

import action "github.com/tdesaules/github-actions/shared/runtime"

func main() {
	action.Main("new-sbom", "syft", []string{"source", "format", "output"})
}
