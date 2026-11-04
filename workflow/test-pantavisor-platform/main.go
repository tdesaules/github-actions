package main

import "github.com/tdesaules/github-actions/shared/pantavisor"

func main() {
	pantavisor.ToolMain("qemu-system-aarch64", []string{"--version"}, pantavisor.TestPlatform)
}
