package main

import "github.com/tdesaules/github-actions/shared/pantavisor"

func main() { pantavisor.ToolMain("python3", []string{"--version"}, pantavisor.PackageComponent) }
