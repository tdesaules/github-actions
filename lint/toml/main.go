package main

import (
	"os"

	"github.com/pelletier/go-toml/v2"
	"github.com/tdesaules/github-actions/shared/check"
)

func validate(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var document map[string]any
	return toml.Unmarshal(data, &document)
}

func main() { check.Main("go-toml/v2 2.4.3", validate) }
