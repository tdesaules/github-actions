// Package rustgtk contains bounded Rust/GTK application qualification primitives.
package rustgtk

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"github.com/tdesaules/github-actions/shared/workflow"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"syscall"
	"time"
)

const Component = "niri-tonic-files"

type Receipt struct {
	SchemaVersion int    `json:"schema_version"`
	SourceSHA     string `json:"source_sha"`
	Version       string `json:"version"`
	Component     string `json:"component"`
	Arch          string `json:"arch"`
	Archive       string `json:"archive"`
	SHA256        string `json:"sha256"`
	RunID         string `json:"run_id"`
	RunAttempt    string `json:"run_attempt"`
	Tested        bool   `json:"tested"`
}
type Inputs struct{ Directory, Output, Version, SourceSHA, Arch, Archive, Receipt, Source, SourceRunID, SourceRunAttempt string }

func Flags(operation string) (Inputs, error) {
	var v Inputs
	f := flag.NewFlagSet(operation, flag.ContinueOnError)
	add := func(p *string, n, d string) {
		f.StringVar(p, n, workflow.Default("INPUT_"+strings.ToUpper(strings.ReplaceAll(n, "-", "_")), d), n)
	}
	add(&v.Directory, "directory", ".")
	defaults := map[string]string{"lint": ".build/lint/rust-gtk.json", "build": ".build/candidate", "test": ".build/qualified"}
	add(&v.Output, "output", defaults[operation])
	if operation != "lint" {
		add(&v.Version, "version", "")
		add(&v.SourceSHA, "source-sha", "")
		add(&v.Arch, "arch", "")
	}
	if operation == "test" {
		add(&v.Archive, "archive", "")
		add(&v.Receipt, "receipt", "")
		add(&v.Source, "source", "candidate")
		add(&v.SourceRunID, "source-run-id", "")
		add(&v.SourceRunAttempt, "source-run-attempt", "")
	}
	err := f.Parse(os.Args[1:])
	if err == nil && f.NArg() != 0 {
		err = fmt.Errorf("unexpected positional arguments")
	}
	return v, err
}
func Identity(v Inputs) error {
	if !regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$`).MatchString(v.Version) {
		return fmt.Errorf("version must be stable SemVer")
	}
	if !regexp.MustCompile(`^[0-9a-f]{40}$`).MatchString(v.SourceSHA) {
		return fmt.Errorf("source-sha must be a full lowercase Git SHA")
	}
	if (v.Arch != "amd64" && v.Arch != "arm64") || v.Arch != runtime.GOARCH {
		return fmt.Errorf("arch must match native CPU %s", runtime.GOARCH)
	}
	return nil
}

// Paths resolves artifact inputs relative to the caller workspace before chdir.
func Paths(v *Inputs) error {
	for _, p := range []*string{&v.Directory, &v.Output, &v.Archive, &v.Receipt} {
		if *p != "" {
			a, e := filepath.Abs(*p)
			if e != nil {
				return e
			}
			*p = a
		}
	}
	if e := os.Chdir(v.Directory); e != nil {
		return e
	}
	// Docker actions start as root; write with the mounted checkout owner's UID.
	if os.Getuid() == 0 {
		info, e := os.Stat(".")
		if e != nil {
			return e
		}
		owner := info.Sys().(*syscall.Stat_t)
		if owner.Uid != 0 {
			if e = syscall.Setgroups([]int{}); e != nil {
				return e
			}
			if e = syscall.Setgid(int(owner.Gid)); e != nil {
				return e
			}
			if e = syscall.Setuid(int(owner.Uid)); e != nil {
				return e
			}
		}
	}
	home, e := os.MkdirTemp("", "rust-gtk-home-")
	if e != nil {
		return e
	}
	return os.Setenv("HOME", home)
}
func Source(sha string) error {
	out, e := workflow.Git("rev-parse", "HEAD")
	if e != nil {
		return e
	}
	if strings.TrimSpace(out) != sha {
		return fmt.Errorf("source HEAD mismatch")
	}
	for _, args := range [][]string{{"diff", "--quiet", "--exit-code", "--"}, {"diff", "--cached", "--quiet", "--exit-code", "HEAD", "--"}} {
		if _, err := workflow.Git(args...); err != nil {
			return fmt.Errorf("source has tracked worktree or index changes: %w", err)
		}
	}
	return nil
}
func Run(dir string, env []string, name string, args ...string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), env...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	fmt.Printf("::group::%s %s\n", name, strings.Join(args, " "))
	defer fmt.Println("::endgroup::")
	if e := cmd.Run(); e != nil {
		return fmt.Errorf("%s: %w", name, e)
	}
	return nil
}
func Hash(path string) (string, error) {
	f, e := os.Open(path)
	if e != nil {
		return "", e
	}
	defer f.Close()
	h := sha256.New()
	if _, e = io.Copy(h, f); e != nil {
		return "", e
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
func JSON(path string, v any) error {
	b, e := json.MarshalIndent(v, "", "  ")
	if e != nil {
		return e
	}
	if e = os.MkdirAll(filepath.Dir(path), 0755); e != nil {
		return e
	}
	return os.WriteFile(path, append(b, '\n'), 0644)
}
func ReadReceipt(path string) (Receipt, error) {
	var r Receipt
	b, e := os.ReadFile(path)
	if e == nil {
		e = json.Unmarshal(b, &r)
	}
	return r, e
}
func Verify(v Inputs, r Receipt) error {
	if e := Identity(v); e != nil {
		return e
	}
	if r.SchemaVersion != 1 || r.SourceSHA != v.SourceSHA || r.Version != v.Version || r.Component != Component || r.Arch != v.Arch || r.Archive != ArchiveName(v) || filepath.Base(v.Archive) != r.Archive || r.RunID == "" || r.RunAttempt == "" {
		return fmt.Errorf("candidate receipt identity mismatch")
	}
	id, attempt := os.Getenv("GITHUB_RUN_ID"), os.Getenv("GITHUB_RUN_ATTEMPT")
	switch v.Source {
	case "", "candidate":
		if r.Tested || v.SourceRunID != "" || v.SourceRunAttempt != "" {
			return fmt.Errorf("candidate mode requires tested:false and current run provenance")
		}
	case "published":
		id, attempt = v.SourceRunID, v.SourceRunAttempt
		if !r.Tested {
			return fmt.Errorf("published mode requires tested:true")
		}
	default:
		return fmt.Errorf("source must be candidate or published")
	}
	if id == "" || attempt == "" || id != r.RunID || attempt != r.RunAttempt {
		return fmt.Errorf("receipt run ID/attempt mismatch or missing expected provenance")
	}
	h, e := Hash(v.Archive)
	if e != nil {
		return e
	}
	if h != r.SHA256 {
		return fmt.Errorf("candidate checksum mismatch")
	}
	return nil
}
func ArchiveName(v Inputs) string {
	return Component + "-" + v.Version + "-linux-" + v.Arch + ".tar.gz"
}
func Outputs(archive, receipt, hash string) error {
	base := workflow.Default("GITHUB_WORKSPACE", "")
	if base != "" {
		if rel, e := filepath.Rel(base, archive); e == nil {
			archive = rel
		}
		if rel, e := filepath.Rel(base, receipt); e == nil {
			receipt = rel
		}
	}
	for _, v := range [][2]string{{"archive", archive}, {"receipt", receipt}, {"sha256", hash}} {
		if e := workflow.Output(v[0], v[1]); e != nil {
			return e
		}
	}
	return nil
}
func Main(run func() error) {
	if len(os.Args) == 2 && os.Args[1] == "--version" {
		fmt.Println("rust-gtk 1")
		return
	}
	workflow.Main("rust-gtk 1", run)
}
