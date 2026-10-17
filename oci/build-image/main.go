package main

import action "github.com/tdesaules/github-actions/shared/runtime"

func main() {
	action.Main("build-image", "buildah", []string{"context", "containerfile", "image", "version", "platform", "target", "labels", "build-args", "secrets", "output"})
}
