package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var imageLine = regexp.MustCompile(`(?m)^(?:  image|        BUILDER_IMAGE): docker://([^\s]+)$`)
var digest = regexp.MustCompile(`@sha256:[a-f0-9]{64}$`)

var allowedBases = map[string]bool{
	"quay.io/fedora/fedora-minimal": true, "quay.io/fedora/fedora": true,
	"docker.io/library/alpine": true, "docker.io/chainguard/wolfi-base": true,
	"docker.io/redhat/ubi10-micro": true, "docker.io/redhat/ubi10-minimal": true,
}

func reportBase(reference, directory string) {
	repository := strings.Split(strings.Split(reference, "@")[0], ":")[0]
	if reference != "scratch" && !allowedBases[repository] {
		fmt.Fprintf(os.Stderr, "WARNING: %s: base/donor %s is outside the base allowlist; migration required\n", directory, reference)
	}
}

func validate(root string) error {
	owners := map[string]string{}
	for _, family := range []string{"oci", "check", "lint", "workflow", "maintenance"} {
		paths, err := filepath.Glob(filepath.Join(root, family, "*", "action.yml"))
		if err != nil {
			return err
		}
		for _, path := range paths {
			directory := filepath.Dir(path)
			container, err := os.ReadFile(filepath.Join(directory, "Containerfile"))
			if family == "workflow" && os.IsNotExist(err) {
				continue
			}
			if err != nil {
				return err
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			match := imageLine.FindSubmatch(data)
			if match == nil {
				return fmt.Errorf("%s: missing image reference", path)
			}
			reference := string(match[1])
			if !digest.MatchString(reference) && !strings.HasSuffix(reference, ":unpublished") {
				return fmt.Errorf("%s: image must be pinned or explicitly unpublished", path)
			}
			image := strings.Split(strings.Split(filepath.Base(reference), ":")[0], "@")[0]
			if family != "lint" && image != family+"-"+filepath.Base(directory) {
				return fmt.Errorf("%s: image is not owned by this action", path)
			}
			if owner, exists := owners[image]; exists {
				return fmt.Errorf("%s and %s own %s", owner, directory, image)
			}
			owners[image] = directory
			if _, err := os.Stat(filepath.Join(directory, "main.go")); err != nil {
				return err
			}
			stages := map[string]bool{}
			final := false
			for _, line := range strings.Split(string(container), "\n") {
				fields := strings.Fields(line)
				if len(fields) > 0 && strings.EqualFold(fields[0], "RUN") {
					for _, field := range fields[1:] {
						if strings.HasPrefix(field, "--mount=") {
							for _, option := range strings.Split(strings.TrimPrefix(field, "--mount="), ",") {
								if strings.HasPrefix(option, "from=") {
									source := strings.TrimPrefix(option, "from=")
									if !stages[source] {
										if !digest.MatchString(source) {
											return fmt.Errorf("%s: unpinned image-backed mount %s", directory, source)
										}
										reportBase(source, directory)
									}
								}
							}
						}
					}
				}
				if len(fields) > 0 && strings.EqualFold(fields[0], "COPY") {
					for _, field := range fields[1:] {
						if strings.HasPrefix(field, "--from=") {
							source := strings.TrimPrefix(field, "--from=")
							if !stages[source] {
								if !digest.MatchString(source) {
									return fmt.Errorf("%s: unpinned external donor %s", directory, source)
								}
								reportBase(source, directory)
							}
						}
					}
				}
				if len(fields) == 0 || !strings.EqualFold(fields[0], "FROM") {
					continue
				}
				if len(fields) != 4 || !strings.EqualFold(fields[2], "AS") {
					return fmt.Errorf("%s: require named stages", directory)
				}
				base := fields[1]
				if base != "scratch" && !stages[base] && !digest.MatchString(base) {
					return fmt.Errorf("%s: unpinned base %s", directory, base)
				}
				if !stages[base] {
					reportBase(base, directory)
				}
				stages[fields[3]] = true
				final = fields[3] == "final"
			}
			if !final {
				return fmt.Errorf("%s: last stage must be named final", directory)
			}
		}
	}
	if len(owners) == 0 {
		return fmt.Errorf("no container actions found")
	}
	fmt.Printf("Validated %d independent image definitions; base-policy warnings are reported separately\n", len(owners))
	return nil
}

func main() {
	directory := flag.String("directory", os.Getenv("INPUT_DIRECTORY"), "catalogue root")
	version := flag.Bool("tool-version", false, "print validator version")
	flag.Parse()
	if *version {
		fmt.Println("catalog 1")
		return
	}
	if flag.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "unexpected positional arguments")
		os.Exit(1)
	}
	if *directory == "" {
		*directory = "."
	}
	if err := validate(*directory); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
