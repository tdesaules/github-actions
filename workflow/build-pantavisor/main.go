package main

import (
	"fmt"
	"github.com/tdesaules/github-actions/shared/pantavisor"
	"github.com/tdesaules/github-actions/shared/workflow"
	"os"
	"path/filepath"
	"strconv"
	"syscall"
)

func run() error {
	directory, project := pantavisor.Input("directory", "."), pantavisor.Input("project", "")
	cold := pantavisor.Input("cold", "true")
	if err := pantavisor.Parse(); err != nil {
		return err
	}
	if *project != "core" && *project != "k0s" && *project != "system" {
		return fmt.Errorf("project must be core, k0s or system")
	}
	if *cold != "true" && *cold != "false" {
		return fmt.Errorf("cold must be true or false")
	}
	if err := pantavisor.Project(*directory); err != nil {
		return err
	}
	root, err := os.Getwd()
	if err != nil {
		return err
	}
	if root != "/work" {
		if _, err := os.Lstat("/work"); os.IsNotExist(err) {
			if err := os.Symlink(root, "/work"); err != nil {
				return err
			}
		} else {
			resolved, e := filepath.EvalSymlinks("/work")
			if e != nil || resolved != root {
				return fmt.Errorf("/work already points to another project")
			}
		}
	}
	info, err := os.Stat(root)
	if err != nil {
		return err
	}
	owner := info.Sys().(*syscall.Stat_t)
	uid, gid := int(owner.Uid), int(owner.Gid)
	if uid == 0 {
		uid, gid = 30000, 30000
	}
	if os.Getuid() == 0 {
		// BitBake must run non-root; use the checkout owner's identity when possible.
		file, err := os.OpenFile("/etc/passwd", os.O_APPEND|os.O_WRONLY, 0600)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintf(file, "\npvbuild:x:%d:%d::/tmp:/bin/bash\n", uid, gid)
		file.Close()
		if err != nil {
			return err
		}
		for _, p := range []string{"artifacts", "artifacts/ci", "artifacts/build", "artifacts/downloads", "artifacts/sstate-cache"} {
			if err := os.MkdirAll(p, 0755); err != nil {
				return err
			}
			if err := os.Chown(p, uid, gid); err != nil {
				return err
			}
		}
		if err := syscall.Setgroups([]int{}); err != nil {
			return err
		}
		if err := syscall.Setgid(gid); err != nil {
			return err
		}
		if err := syscall.Setuid(uid); err != nil {
			return err
		}
	}
	os.Setenv("HOME", "/tmp")
	os.Setenv("USER", "pvbuild")
	os.Setenv("LOGNAME", "pvbuild")
	os.Setenv("PV_BUILD_UID", strconv.Itoa(uid))
	if err := pantavisor.Python("/opt/pantavisor/build.py", *project, *cold); err != nil {
		return err
	}
	return workflow.Output("deploy-directory", "artifacts/ci/deploy")
}
func main() {
	// Docker actions start this executable as PID 1. A long BitBake build
	// creates orphaned descendants that os/exec cannot reap on its own.
	// Hand PID 1 to the distribution's init, keeping /entrypoint as the
	// image interface. Its child has a different PID, so this happens once.
	if os.Getpid() == 1 {
		init := "/usr/libexec/catatonit/catatonit"
		args := append([]string{init, "--", "/entrypoint"}, os.Args[1:]...)
		if err := syscall.Exec(init, args, os.Environ()); err != nil {
			fmt.Fprintln(os.Stderr, "start build process reaper:", err)
			os.Exit(1)
		}
	}
	if len(os.Args) == 2 && os.Args[1] == "--tool-version" {
		if err := pantavisor.Run("perl", "-e", `use Thread::Queue; use Data::Dumper; use File::Compare; use File::Copy; use FindBin; use open ":std";`); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}
	pantavisor.ToolMain("kas", []string{"--version"}, run)
}
