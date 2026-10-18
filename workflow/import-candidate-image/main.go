package main

import (
	"crypto/sha256"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
)

func execute(args []string) error {
	flags := flag.NewFlagSet("import-candidate-image", flag.ContinueOnError)
	archive := flags.String("archive", os.Getenv("INPUT_ARCHIVE"), "candidate OCI archive")
	repository := flags.String("repository", os.Getenv("INPUT_REPOSITORY"), "repository name inside the isolated candidate registry")
	defaultPort := os.Getenv("INPUT_HOST-PORT")
	if defaultPort == "" {
		defaultPort = "5000"
	}
	portValue := flags.String("host-port", defaultPort, "candidate registry port exposed to the runner")
	if err := flags.Parse(args); err != nil {
		return err
	}
	port, err := strconv.Atoi(*portValue)
	if err != nil {
		return err
	}
	if flags.NArg() != 0 || *archive == "" || !regexp.MustCompile(`^[a-z0-9]+(?:[._-][a-z0-9]+)*$`).MatchString(*repository) || port < 1 || port > 65535 {
		return errors.New("archive, simple repository name and valid host-port are required")
	}
	path, err := filepath.Abs(*archive)
	if err != nil {
		return err
	}
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return errors.New("archive must be a regular file")
	}
	source := "oci-archive:" + path
	inspect := exec.Command("/usr/bin/skopeo", "inspect", "--raw", source)
	inspect.Stderr = os.Stderr
	manifest, err := inspect.Output()
	if err != nil {
		return err
	}
	digest := fmt.Sprintf("sha256:%x", sha256.Sum256(manifest))
	// HTTP is restricted to this fixed, private GitHub Actions service alias.
	// Registry publication elsewhere continues to require TLS verification.
	destination := "docker://candidate-registry:5000/" + *repository + "@" + digest
	copy := exec.Command("/usr/bin/skopeo", "copy", "--preserve-digests", "--retry-times=3", "--dest-tls-verify=false", source, destination)
	copy.Stdout, copy.Stderr = os.Stdout, os.Stderr
	if err := copy.Run(); err != nil {
		return err
	}
	image := fmt.Sprintf("localhost:%d/%s@%s", port, *repository, digest)
	fmt.Println(image)
	if output := os.Getenv("GITHUB_OUTPUT"); output != "" {
		file, err := os.OpenFile(output, os.O_APPEND|os.O_WRONLY, 0600)
		if err != nil {
			return err
		}
		_, writeErr := fmt.Fprintf(file, "image=%s\ndigest=%s\n", image, digest)
		closeErr := file.Close()
		if writeErr != nil {
			return writeErr
		}
		return closeErr
	}
	return nil
}

func main() {
	if len(os.Args) == 2 && os.Args[1] == "--tool-version" {
		fmt.Println("import-candidate-image 1")
		return
	}
	if err := execute(os.Args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return
		}
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
