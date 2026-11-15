package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"runtime"
	"strconv"
	"syscall"
)

const toolVersion = "build-atomic-image 1 (Docker 29.9.3, Buildx v0.39.0, BuildKit v0.35.0)"

type options struct {
	Context, Containerfile, Image, Version, Platform, Target, Labels, BuildArgs, Output string
}

func parse(args []string) (options, error) {
	var o options
	f := flag.NewFlagSet("build-atomic-image", flag.ContinueOnError)
	f.SetOutput(io.Discard)
	f.StringVar(&o.Context, "context", ".", "workspace-relative context")
	f.StringVar(&o.Containerfile, "containerfile", "Containerfile", "workspace-relative Containerfile")
	f.StringVar(&o.Image, "image", "", "untagged image name")
	f.StringVar(&o.Version, "version", "", "image tag")
	f.StringVar(&o.Platform, "platform", "", "native Linux platform")
	f.StringVar(&o.Target, "target", "", "optional build stage")
	f.StringVar(&o.Labels, "labels", "{}", "trusted JSON labels")
	f.StringVar(&o.BuildArgs, "build-args", "{}", "non-secret JSON build arguments")
	f.StringVar(&o.Output, "output", "image.oci.tar", "new workspace-relative archive")
	if err := f.Parse(args); err != nil || f.NArg() != 0 {
		return o, errors.New("invalid action arguments; use only the documented named inputs")
	}
	return o, nil
}

func outputs(r result) error {
	text := "archive=" + r.Archive + "\narchive-sha256=" + r.SHA256 + "\nsize=" + strconv.FormatInt(r.Size, 10) + "\n"
	if path := os.Getenv("GITHUB_OUTPUT"); path != "" {
		f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0600)
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
		workspace := os.Getenv("GITHUB_WORKSPACE")
		if workspace == "" {
			workspace, err = os.Getwd()
			if err != nil {
				return err
			}
		}
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()
		r, err := build(ctx, workspace, runtime.GOARCH, o, executeDocker)
		if err != nil {
			return err
		}
		return outputs(r)
	}()
	if err != nil {
		fmt.Fprintln(os.Stderr, "Atomic image build failed:", err)
		os.Exit(1)
	}
}
