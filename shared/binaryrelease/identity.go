package binaryrelease

import (
	"fmt"
	"os"
	"syscall"
)

type identity struct{ uid, gid int }

func runnerIdentity(output, workspace string, lookup func(string) (identity, error)) (identity, error) {
	path := workspace
	if output != "" {
		path = output
	}
	owner, err := lookup(path)
	if err != nil {
		return identity{}, fmt.Errorf("cannot determine runner output ownership")
	}
	return owner, nil
}

// Docker starts as root; adopt the runner identity before creating any files.
// A configured output is authoritative. Do not silently fall back on lookup error.
func adoptRunnerIdentity() error {
	if os.Geteuid() != 0 {
		return nil
	}
	workspace, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("cannot resolve workspace")
	}
	owner, err := runnerIdentity(os.Getenv("GITHUB_OUTPUT"), workspace, func(path string) (identity, error) {
		info, e := os.Stat(path)
		if e != nil {
			return identity{}, e
		}
		stat, ok := info.Sys().(*syscall.Stat_t)
		if !ok {
			return identity{}, fmt.Errorf("unsupported file ownership")
		}
		return identity{int(stat.Uid), int(stat.Gid)}, nil
	})
	if err != nil {
		return err
	}
	if owner.uid == 0 {
		return nil
	}
	if err = syscall.Setgroups([]int{}); err != nil {
		return fmt.Errorf("cannot clear supplementary groups")
	}
	if err = syscall.Setgid(owner.gid); err != nil {
		return fmt.Errorf("cannot adopt runner group")
	}
	if err = syscall.Setuid(owner.uid); err != nil {
		return fmt.Errorf("cannot adopt runner user")
	}
	return nil
}
