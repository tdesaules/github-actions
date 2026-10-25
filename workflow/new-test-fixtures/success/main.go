// A network-independent native RUN fixture for the narrowly scoped Buildah launcher.
package main

import (
	"os"
)

func main() {
	if err := os.MkdirAll("/runtime/tmp", 0777); err != nil {
		panic(err)
	}
	if err := os.Chmod("/runtime/tmp", os.ModeSticky|0777); err != nil {
		panic(err)
	}
	if err := os.WriteFile("/runtime/proof", []byte("native RUN completed\n"), 0644); err != nil {
		panic(err)
	}
}
