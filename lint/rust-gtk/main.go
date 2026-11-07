package main

import (
	"encoding/json"
	"fmt"
	"github.com/tdesaules/github-actions/shared/rustgtk"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
)

func lockCoherent(dir string) error {
	var pkg struct {
		Name            string            `json:"name"`
		Version         string            `json:"version"`
		Dependencies    map[string]string `json:"dependencies"`
		DevDependencies map[string]string `json:"devDependencies"`
	}
	b, e := os.ReadFile(filepath.Join(dir, "package.json"))
	if e != nil {
		return e
	}
	if e = json.Unmarshal(b, &pkg); e != nil {
		return e
	}
	var lock struct {
		Name     string `json:"name"`
		Version  string `json:"version"`
		Packages map[string]struct {
			Dependencies    map[string]string `json:"dependencies"`
			DevDependencies map[string]string `json:"devDependencies"`
		} `json:"packages"`
	}
	b, e = os.ReadFile(filepath.Join(dir, "package-lock.json"))
	if e != nil {
		return e
	}
	if e = json.Unmarshal(b, &lock); e != nil {
		return e
	}
	p, ok := lock.Packages[""]
	if !ok || pkg.Name != lock.Name || pkg.Version != lock.Version || !reflect.DeepEqual(pkg.Dependencies, p.Dependencies) || !reflect.DeepEqual(pkg.DevDependencies, p.DevDependencies) {
		return fmt.Errorf("%s package lock mismatch", dir)
	}
	return nil
}
func lint(v rustgtk.Inputs) error {
	if e := rustgtk.Run("", nil, "cargo", "fmt", "--check"); e != nil {
		return e
	}
	if e := rustgtk.Run("", nil, "cargo", "clippy", "--all-targets", "--locked", "-j2", "--", "-D", "warnings"); e != nil {
		return e
	}
	for _, dir := range []string{"plugins/filen", "plugins/google-drive"} {
		if e := lockCoherent(dir); e != nil {
			return e
		}
	}
	return filepath.WalkDir(".", func(p string, d fs.DirEntry, e error) error {
		if e != nil {
			return e
		}
		if d.IsDir() {
			if d.Name() == "target" || d.Name() == "node_modules" || strings.HasPrefix(d.Name(), ".") && p != "." {
				return filepath.SkipDir
			}
			return nil
		}
		switch {
		case strings.HasPrefix(p, "plugins/") && filepath.Ext(p) == ".mjs":
			return rustgtk.Run("", nil, "node", "--check", p)
		case filepath.Ext(p) == ".py":
			return rustgtk.Run("", nil, "python3", "-c", "import pathlib,sys; p=pathlib.Path(sys.argv[1]); compile(p.read_bytes(),str(p),'exec')", p)
		case filepath.Ext(p) == ".desktop":
			return rustgtk.Run("", nil, "desktop-file-validate", p)
		}
		return nil
	})
}
func run() error {
	v, e := rustgtk.Flags("lint")
	if e != nil {
		return e
	}
	if e = rustgtk.Paths(&v); e != nil {
		return e
	}
	e = lint(v)
	report := map[string]any{"schema_version": 1, "passed": e == nil}
	if e != nil {
		report["error"] = e.Error()
	}
	if re := rustgtk.JSON(v.Output, report); re != nil {
		return re
	}
	return e
}
func main() { rustgtk.Main(run) }
