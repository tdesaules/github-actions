// Package pantavisor supplies argument, process and filesystem primitives to
// dedicated Pantavisor actions. No action accepts a command or operation input.
package pantavisor

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"github.com/tdesaules/github-actions/shared/workflow"
)

func Input(name, fallback string) *string {
	return flag.String(name, workflow.Default("INPUT_"+strings.ToUpper(name), fallback), name)
}
func Parse() error {
	flag.Parse()
	if flag.NArg() != 0 {
		return fmt.Errorf("unexpected positional arguments")
	}
	return nil
}
func Project(directory string) error {
	if err := os.Chdir(directory); err != nil {
		return err
	}
	root, err := os.Getwd()
	if err != nil {
		return err
	}
	count := 0
	if v := os.Getenv("GIT_CONFIG_COUNT"); v != "" {
		count, err = strconv.Atoi(v)
		if err != nil || count < 0 || count > 1024 {
			return fmt.Errorf("invalid Git config count")
		}
	}
	for k, v := range map[string]string{fmt.Sprintf("GIT_CONFIG_KEY_%d", count): "safe.directory", fmt.Sprintf("GIT_CONFIG_VALUE_%d", count): root, "GIT_CONFIG_COUNT": strconv.Itoa(count + 1), "PYTHONDONTWRITEBYTECODE": "1"} {
		if err := os.Setenv(k, v); err != nil {
			return err
		}
	}
	return nil
}
func Local(value string) error {
	if value == "" || !filepath.IsLocal(value) || strings.ContainsAny(value, "\x00\r\n") {
		return fmt.Errorf("expected a project-relative path, got %q", value)
	}
	return nil
}
func Run(binary string, args ...string) error {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	cmd := exec.CommandContext(ctx, binary, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	return cmd.Run()
}
func Python(args ...string) error           { return Run("python3", append([]string{"-B"}, args...)...) }
func Main(version string, run func() error) { workflow.Main(version, run) }
func ToolMain(binary string, args []string, run func() error) {
	if len(os.Args) == 2 && os.Args[1] == "--tool-version" {
		if err := Run(binary, args...); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	workflow.Main(binary, run)
}

func PackageComponent() error {
	directory, deploy, output := Input("directory", "."), Input("deploy-directory", "artifacts/ci/deploy"), Input("output", "dist/component")
	if err := Parse(); err != nil {
		return err
	}
	if err := Project(*directory); err != nil {
		return err
	}
	for _, p := range []string{*deploy, *output} {
		if err := Local(p); err != nil {
			return err
		}
	}
	if err := Python("build/package.py", "--deploy-dir", *deploy, "--output", *output); err != nil {
		return err
	}
	return workflow.Output("component", *output)
}
func PackageBSP() error {
	directory, deploy, output := Input("directory", "."), Input("deploy-directory", "artifacts/ci/deploy"), Input("output", "dist/bsp")
	if err := Parse(); err != nil {
		return err
	}
	if err := Project(*directory); err != nil {
		return err
	}
	for _, p := range []string{*deploy, *output} {
		if err := Local(p); err != nil {
			return err
		}
	}
	if err := Python("-m", "tools.pvbuild", "package-bsp", "--deploy-dir", *deploy, "--output", *output); err != nil {
		return err
	}
	return workflow.Output("component", *output)
}
func Assemble() error {
	directory, bsp, core, cluster, profile, output := Input("directory", "."), Input("bsp", "dist/bsp"), Input("core", "dist/core"), Input("k0s", ""), Input("profile", "base"), Input("output", "dist/platform")
	if err := Parse(); err != nil {
		return err
	}
	if err := Project(*directory); err != nil {
		return err
	}
	if *profile != "base" && *profile != "k0s" {
		return fmt.Errorf("invalid profile")
	}
	for _, p := range []string{*bsp, *core, *output} {
		if err := Local(p); err != nil {
			return err
		}
	}
	args := []string{"-m", "tools.pvbuild", "assemble", "--bsp", *bsp, "--core", *core, "--profile", *profile, "--output", *output}
	if *profile == "k0s" {
		if err := Local(*cluster); err != nil {
			return err
		}
		args = append(args, "--k0s", *cluster)
	} else if *cluster != "" {
		return fmt.Errorf("base profile must not receive k0s")
	}
	if err := Python(args...); err != nil {
		return err
	}
	return workflow.Output("bundle", *output)
}
func TestRootfs() error {
	directory, component := Input("directory", "."), Input("component", "dist/component")
	if err := Parse(); err != nil {
		return err
	}
	if err := Project(*directory); err != nil {
		return err
	}
	if err := Local(*component); err != nil {
		return err
	}
	return Python("build/smoke.py", *component)
}
func TestPlatform() error {
	directory, bundle, output, reports := Input("directory", "."), Input("bundle", "dist/platform"), Input("output", "artifacts/qualification"), Input("reports", "artifacts/public-reports")
	if err := Parse(); err != nil {
		return err
	}
	if err := Project(*directory); err != nil {
		return err
	}
	for _, p := range []string{*bundle, *output, *reports} {
		if err := Local(p); err != nil {
			return err
		}
	}
	return Python("/opt/pantavisor/qualify.py", *bundle, *output, *reports)
}
