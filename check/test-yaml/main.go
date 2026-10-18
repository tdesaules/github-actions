package main

import (
	"errors"
	"io"
	"os"

	"github.com/tdesaules/github-actions/shared/check"
	"go.yaml.in/yaml/v3"
)

func validate(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	decoder := yaml.NewDecoder(f)
	for {
		var document any
		err := decoder.Decode(&document)
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
	}
}

func main() { check.Main("go.yaml.in/yaml/v3 3.0.5", validate) }
