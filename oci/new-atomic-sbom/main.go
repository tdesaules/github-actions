package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"
	"time"
)

const toolVersion = "new-atomic-sbom 1 / Syft 1.54.1"
const syftRSSBudget = 7 * 1024 * 1024 * 1024

type options struct {
	Archive, SHA256, Reference, Architecture, Revision, Output, Binding, Username string
}

func parse(args []string) (options, error) {
	var o options
	f := flag.NewFlagSet("new-atomic-sbom", flag.ContinueOnError)
	f.SetOutput(io.Discard)
	f.StringVar(&o.Archive, "archive", "", "workspace-relative native OCI archive")
	f.StringVar(&o.SHA256, "sha256", "", "required archive checksum")
	f.StringVar(&o.Reference, "reference", "", "immutable registry reference")
	f.StringVar(&o.Architecture, "architecture", "", "amd64 or arm64; static cross-architecture scans supported")
	f.StringVar(&o.Revision, "revision", "", "optional expected OCI source revision label")
	f.StringVar(&o.Output, "output", ".build/security/atomic.spdx.json", "new workspace-relative SPDX file")
	f.StringVar(&o.Binding, "binding", "", "new source-binding report; defaults to output + .binding.json")
	if err := f.Parse(args); err != nil || f.NArg() != 0 {
		return o, errors.New("invalid named action inputs")
	}
	return o, nil
}

func outputs(o options, b binding) error {
	text := fmt.Sprintf("sbom=%s\nsource-binding=%s\nsbom-sha256=%s\nmanifest-digest=%s\narchitecture=%s\nrpm-packages=%d\nrpm-covered=%d\n",
		o.Output, o.Binding, b.SBOM.SHA256, b.Native.Digest, b.Architecture, b.RPM.Installed, b.RPM.Covered)
	if path := os.Getenv("GITHUB_OUTPUT"); path != "" {
		f, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0600)
		if err != nil {
			return err
		}
		_, err = f.WriteString(text)
		return errors.Join(err, f.Close())
	}
	_, err := fmt.Print(text)
	return err
}

func main() {
	if len(os.Args) == 2 && (os.Args[1] == "--tool-version" || os.Args[1] == "--version") {
		fmt.Println(toolVersion)
		return
	}
	err := func() error {
		o, err := parse(os.Args[1:])
		if err != nil {
			return err
		}
		o.Username = os.Getenv("INPUT_REGISTRY_USERNAME")
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()
		ctx, cancel := context.WithTimeout(ctx, 45*time.Minute)
		defer cancel()
		workspace, err := os.Getwd()
		if err != nil {
			return err
		}
		b, normalized, err := scan(ctx, workspace, o, os.Getenv("INPUT_REGISTRY_PASSWORD"), execute)
		if err != nil {
			return err
		}
		return outputs(normalized, b)
	}()
	if err != nil {
		fmt.Fprintln(os.Stderr, "Atomic SBOM:", err)
		os.Exit(1)
	}
}
