// update-tool-checksums is the fixed post-upgrade task for public Go tool sources.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

var source = regexp.MustCompile(`(compile-tool\.sh\s+)((?:github\.com|golang\.org|google\.golang\.org|go\.podman\.io|oras\.land)/[A-Za-z0-9_./-]+)(\s+)(v[0-9][A-Za-z0-9.+-]*)(\s+)(h1:[A-Za-z0-9+/]{43}=)`)
var checksum = regexp.MustCompile(`^h1:[A-Za-z0-9+/]{43}=$`)

func download(module, version string) (string, error) {
	cache, err := os.MkdirTemp("", "tool-checksum-cache-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(cache)
	cmd := exec.Command("go", "mod", "download", "-json", module+"@"+version)
	// Renovate sanitizes child environments, dropping image-level Go cache
	// variables. GitHub's injected HOME may be unwritable for the action UID.
	// Work outside the checkout so downloads cannot modify its go.sum either.
	cmd.Dir = cache
	// Do not inherit repository-local proxies, exclusions or workspace settings.
	env := []string{}
	settings := map[string]string{"GOSUMDB": "sum.golang.org", "GOPROXY": "https://proxy.golang.org", "GONOSUMDB": "", "GONOPROXY": "", "GOPRIVATE": "", "GOWORK": "off", "GOFLAGS": "", "GOTOOLCHAIN": "local", "GOPATH": filepath.Join(cache, "go"), "GOMODCACHE": filepath.Join(cache, "modules"), "GOCACHE": filepath.Join(cache, "build")}
	for _, entry := range os.Environ() {
		key, _, _ := strings.Cut(entry, "=")
		if _, replace := settings[key]; !replace {
			env = append(env, entry)
		}
	}
	for key, value := range settings {
		env = append(env, key+"="+value)
	}
	cmd.Env = env
	data, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("checksum lookup failed for %s@%s: %w", module, version, err)
	}
	var result struct{ Path, Version, Sum, Error string }
	if err := json.Unmarshal(data, &result); err != nil {
		return "", err
	}
	if result.Path != module || result.Version != version || result.Error != "" || !checksum.MatchString(result.Sum) {
		return "", fmt.Errorf("invalid checksum response for %s@%s", module, version)
	}
	return result.Sum, nil
}

func update(data string, lookup func(string, string) (string, error)) (string, error) {
	var failure error
	result := source.ReplaceAllStringFunc(data, func(match string) string {
		if failure != nil {
			return match
		}
		parts := source.FindStringSubmatch(match)
		sum, err := lookup(parts[2], parts[4])
		if err != nil {
			failure = err
			return match
		}
		if !checksum.MatchString(sum) {
			failure = fmt.Errorf("invalid tool checksum")
			return match
		}
		return strings.TrimSuffix(match, parts[6]) + sum
	})
	if failure != nil {
		return "", failure
	}
	return result, nil
}

func run() error {
	if len(os.Args) != 1 {
		return fmt.Errorf("no arguments accepted")
	}
	files := []string{}
	for _, pattern := range []string{"oci/*/Containerfile", "check/*/Containerfile", "lint/*/Containerfile", "workflow/*/Containerfile", "maintenance/*/Containerfile", "shared/runtime/compile-*.sh"} {
		paths, err := filepath.Glob(pattern)
		if err != nil {
			return err
		}
		files = append(files, paths...)
	}
	cache := map[string]string{}
	lookup := func(module, version string) (string, error) {
		key := module + "@" + version
		if sum, ok := cache[key]; ok {
			return sum, nil
		}
		sum, err := download(module, version)
		if err == nil {
			cache[key] = sum
		}
		return sum, err
	}
	updates := map[string]string{}
	for _, path := range files {
		info, err := os.Lstat(path)
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("non-regular tool recipe: %s", path)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		updated, err := update(string(data), lookup)
		if err != nil {
			return err
		}
		if updated != string(data) {
			updates[path] = updated
		}
	}
	// Resolve every checksum successfully before writing any recipe.
	for path, data := range updates {
		if err := os.WriteFile(path, []byte(data), 0644); err != nil {
			return err
		}
	}
	return nil
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
