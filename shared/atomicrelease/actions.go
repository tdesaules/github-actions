package atomicrelease

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/tdesaules/github-actions/shared/workflow"
)

type Options struct {
	Directory, Source, Version, Repository, Image, ISO, Plan, Config, Output string
}

func CLI(name string, args []string, operation func(Options) error) error {
	if len(args) == 1 && (args[0] == "--tool-version" || args[0] == "--version") {
		fmt.Println(name + " 1")
		return nil
	}
	f := flag.NewFlagSet(name, flag.ContinueOnError)
	f.SetOutput(io.Discard)
	o := Options{}
	f.StringVar(&o.Directory, "qualified-directory", ".build/qualified", "qualified bundle")
	f.StringVar(&o.Source, "source-sha", "", "qualified source")
	f.StringVar(&o.Version, "version", "", "release version")
	f.StringVar(&o.Repository, "repository", "", "GitHub owner/repository")
	f.StringVar(&o.Image, "image", "", "published immutable OS index")
	f.StringVar(&o.ISO, "iso", "", "published immutable installer")
	f.StringVar(&o.Plan, "plan", ".build/plan.json", "preparation plan")
	f.StringVar(&o.Config, "config", "cliff.toml", "git-cliff notes configuration")
	f.StringVar(&o.Output, "output", ".build/atomic-qualified.json", "bundle output")
	if err := f.Parse(args); err != nil || f.NArg() != 0 {
		return fmt.Errorf("invalid action arguments")
	}
	return operation(o)
}
func VerifyAction(o Options) error {
	b, err := Verify(o.Directory, o.Source, o.Version, os.Getenv("GITHUB_RUN_ID"), os.Getenv("GITHUB_RUN_ATTEMPT"))
	if err != nil {
		return err
	}
	files := b.ISO.Files
	if err = VerifyPayloads(o.Directory, files, b.ISO); err != nil {
		return err
	}
	if err = Write(o.Output, b); err != nil {
		return err
	}
	for k, v := range map[string]any{"bundle": o.Output, "iso_directory": filepath.Join(o.Directory, "amd64"), "iso_files": files,
		"amd64_archive": filepath.Join(o.Directory, "amd64", b.AMD64.Archive), "arm64_archive": filepath.Join(o.Directory, "arm64", b.ARM64.Archive),
		"amd64_sha256": b.AMD64.SHA256, "arm64_sha256": b.ARM64.SHA256,
		"iso_file": b.ISO.Archive, "iso_sha256": b.ISO.SHA256} {
		if err = workflow.Output(k, v); err != nil {
			return err
		}
	}
	return nil
}
func PrepareAction(o Options) error {
	if os.Getenv("GITHUB_EVENT_NAME") != "workflow_dispatch" || os.Getenv("GITHUB_REF") != "refs/heads/main" {
		return fmt.Errorf("preparation requires manual main dispatch")
	}
	var p struct {
		SchemaVersion  int    `json:"schema_version"`
		Mode           string `json:"mode"`
		SourceSHA      string `json:"source_sha"`
		Version        string `json:"version"`
		HasRelease     bool   `json:"has_release"`
		InitialRelease bool   `json:"initial_release"`
	}
	if err := Decode(o.Plan, &p); err != nil {
		return err
	}
	head, err := git("rev-parse", "HEAD")
	if err != nil {
		return err
	}
	if p.SchemaVersion != 1 || p.Mode != "prepare" || p.SourceSHA != head || o.Source != head {
		return fmt.Errorf("preparation plan must identify checkout source")
	}
	version, changed, err := ReleaseVersion(p.InitialRelease)
	if err != nil {
		return err
	}
	if p.HasRelease != changed || (changed && p.Version != version) {
		return fmt.Errorf("preparation plan disagrees with source release policy")
	}
	if !changed {
		return workflow.Output("changed", "false")
	}
	b, err := Verify(o.Directory, head, version, os.Getenv("GITHUB_RUN_ID"), os.Getenv("GITHUB_RUN_ATTEMPT"))
	if err != nil {
		return err
	}
	e := Evidence{SchemaVersion: 1, SourceSHA: head, Version: version, Image: o.Image, ISO: o.ISO, ISOFile: b.ISO.Archive, ISOSHA256: b.ISO.SHA256, Architectures: []string{"amd64", "arm64"}, Qualified: b}
	e.ISOFiles = b.ISO.Files
	if err = VerifyPayloads(o.Directory, e.ISOFiles, b.ISO); err != nil {
		return err
	}
	a, err := NewRemote(o.Repository, os.Getenv("INPUT_TOKEN"), os.Getenv("INPUT_REGISTRY_PASSWORD"), os.Getenv("INPUT_REGISTRY_USERNAME"))
	if err != nil {
		return err
	}
	if err = a.VerifyAppToken(); err != nil {
		return err
	}
	if _, err = a.VerifyPublished(e); err != nil {
		return err
	}
	status, err := git("status", "--porcelain", "--untracked-files=no")
	if err != nil {
		return err
	}
	if status != "" {
		return fmt.Errorf("tracked checkout and index must be clean")
	}
	remote := "https://github.com/" + o.Repository + ".git"
	recheck := func() error {
		refs, err := authenticatedGit(a.Token, "ls-remote", remote, "refs/heads/main", "refs/tags/"+version)
		if err != nil {
			return err
		}
		return CheckRemote(refs, head)
	}
	if err = recheck(); err != nil {
		return err
	}
	notes, err := cliff("--config", o.Config, "--tag", version)
	if err != nil {
		return err
	}
	if err = ValidateNotes(notes, version); err != nil {
		return err
	}
	// Existing untracked paths cannot silently become release metadata.
	for _, name := range []string{"CHANGELOG.md", ".github/atomic-release.json"} {
		if _, err := os.Lstat(name); err == nil {
			if _, err = git("ls-files", "--error-unmatch", "--", name); err != nil {
				return fmt.Errorf("untracked release metadata already exists")
			}
		}
	}
	if err = Write(".github/atomic-release.json", e); err != nil {
		return err
	}
	if err = os.WriteFile("CHANGELOG.md", []byte(notes+"\n"), 0644); err != nil {
		return err
	}
	if _, err = git("add", "--", "CHANGELOG.md", ".github/atomic-release.json"); err != nil {
		return err
	}
	identity := []string{"-c", "commit.gpgSign=false", "-c", "tag.gpgSign=false", "-c", "user.name=github-actions[bot]", "-c", "user.email=41898282+github-actions[bot]@users.noreply.github.com"}
	if _, err = git(append(identity, "commit", "-m", "chore(release): prepare "+version, "-m", "Bind the qualified Atomic OS and installer digests to their exact source and release notes.")...); err != nil {
		return err
	}
	final, err := git("rev-parse", "HEAD")
	if err != nil {
		return err
	}
	if err = ValidateMetadata(head, final, version); err != nil {
		return err
	}
	if err = recheck(); err != nil {
		return err
	}
	if _, err = git(append(identity, "tag", "-a", version, "-m", "Qualified Atomic OS release "+version, final)...); err != nil {
		return err
	}
	if _, err = authenticatedGit(a.Token, "push", "--atomic", "--force-with-lease=refs/heads/main:"+head, remote, final+":refs/heads/main", "refs/tags/"+version+":refs/tags/"+version); err != nil {
		return err
	}
	for k, v := range map[string]string{"changed": "true", "tag": version, "source_sha": head, "final_sha": final, "image": e.Image, "iso": e.ISO} {
		if err = workflow.Output(k, v); err != nil {
			return err
		}
	}
	return nil
}
func CheckRemote(refs, source string) error {
	if strings.TrimSpace(refs) != source+"\trefs/heads/main" {
		return fmt.Errorf("main advanced or release tag already exists")
	}
	return nil
}
func PublishAction(o Options) error {
	if os.Getenv("GITHUB_EVENT_NAME") != "push" || os.Getenv("GITHUB_REF") != "refs/tags/"+o.Version {
		return fmt.Errorf("publication requires exact stable tag push")
	}
	e, final, err := ReadTag(o.Version, o.Repository)
	if err != nil {
		return err
	}
	// Tag tests requalify published bytes; OCI re-export may change the archive
	// checksum, so compare native manifest identity, not transport tar identity.
	b, err := verify(o.Directory, e.SourceSHA, e.Version, os.Getenv("GITHUB_RUN_ID"), os.Getenv("GITHUB_RUN_ATTEMPT"), e.Qualified.AMD64.SHA256)
	if err != nil {
		return err
	}
	if err = VerifyPayloads(o.Directory, e.ISOFiles, b.ISO); err != nil {
		return err
	}
	if b.AMD64.ManifestDigest != e.Qualified.AMD64.ManifestDigest || b.ARM64.ManifestDigest != e.Qualified.ARM64.ManifestDigest ||
		b.ISO.SHA256 != e.ISOSHA256 || b.ISO.Archive != e.ISOFile ||
		b.AMD64.Reference != e.Image || b.ARM64.Reference != e.Image {
		return fmt.Errorf("tag qualification differs from prepared immutable release")
	}
	a, err := NewRemote(o.Repository, os.Getenv("INPUT_TOKEN"), os.Getenv("INPUT_REGISTRY_PASSWORD"), os.Getenv("INPUT_REGISTRY_USERNAME"))
	if err != nil {
		return err
	}
	raw, err := a.VerifyPublished(e)
	if err != nil {
		return err
	}
	// Older releases publish their record without rolling channels back.
	// Serialize consumer release jobs to protect latest-version selection.
	refs, err := authenticatedGit(a.Token, "ls-remote", "https://github.com/"+o.Repository+".git", "refs/tags/*")
	if err != nil {
		return err
	}
	latest, err := TagStatus(refs, e.Version, final)
	if err != nil {
		return err
	}
	notes, err := git("show", final+":CHANGELOG.md")
	if err != nil {
		return err
	}
	notes, err = ReleaseNotes(notes, e.Version)
	if err != nil {
		return err
	}
	if _, err = a.CheckRelease(e, final, notes); err != nil {
		return err
	}
	// Each step is idempotent. Registry channels cannot be changed atomically
	// together; a failed second update is repaired by retrying this same tag.
	if latest {
		if err = a.Promote(raw); err != nil {
			return err
		}
	}
	releaseURL, err := a.Release(e, final, notes, latest)
	if err != nil {
		return err
	}
	return workflow.Output("release_url", releaseURL)
}
func TagStatus(refs, version, final string) (bool, error) {
	found := false
	latest := true
	for _, line := range strings.Split(refs, "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 {
			continue
		}
		tag := strings.TrimPrefix(fields[1], "refs/tags/")
		if tag == version+"^{}" {
			found = fields[0] == final
		}
		if Stable.MatchString(tag) && Compare(tag, version) > 0 {
			latest = false
		}
	}
	if !found {
		return false, fmt.Errorf("remote annotated tag does not identify release metadata")
	}
	return latest, nil
}
func ReleaseNotes(changelog, version string) (string, error) {
	start := strings.Index(changelog, "## ["+version+"] - ")
	if start < 0 {
		return "", fmt.Errorf("release notes missing")
	}
	body := changelog[start:]
	if end := strings.Index(body[1:], "\n## ["); end >= 0 {
		body = body[:end+1]
	}
	// Keep comparison definitions available when extracting one release.
	for _, line := range strings.Split(changelog, "\n") {
		if strings.HasPrefix(line, "["+version+"]: ") {
			body = strings.TrimSpace(body) + "\n\n" + line
			break
		}
	}
	return strings.TrimSpace(body), nil
}
