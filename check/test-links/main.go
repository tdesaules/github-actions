package main

import (
	"flag"
	"os"

	"github.com/tdesaules/github-actions/shared/check"
)

func main() {
	var config string
	check.Main("lychee 0.24.2", func(path string) error {
		args := []string{"--no-progress", "--timeout", "30", "--max-retries", "2", "--max-concurrency", "4"}
		if config != "" {
			args = append(args, "--config", config)
		}
		return check.External("/usr/local/bin/lychee", args...)(path)
	}, func(flags *flag.FlagSet) {
		flags.StringVar(&config, "config", os.Getenv("INPUT_CONFIG"), "optional Lychee TOML configuration")
	})
}
