package main

import action "github.com/tdesaules/github-actions/shared/runtime"

func main() {
	action.Main("test-vulnerabilities", "grype", []string{"sbom", "fail-on", "update-database", "output"})
}
