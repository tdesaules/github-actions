// Annotate the qualified source itself with transport-independent image identities.
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

func tagApplications() error {
	version, rev := os.Getenv("VERSION"), os.Getenv("GITHUB_SHA")
	if os.Getenv("GITHUB_REF") != "refs/heads/main" || !regexp.MustCompile(`^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)$`).MatchString(version) {
		return fmt.Errorf("invalid preparation context")
	}
	evidence := map[string]string{}
	var reused map[string]string
	if raw := os.Getenv("REUSED_DIGESTS"); raw != "" {
		if err := json.Unmarshal([]byte(raw), &reused); err != nil {
			return err
		}
	}
	var published map[string]string
	if e := json.Unmarshal([]byte(os.Getenv("APP_DIGESTS")), &published); e != nil || len(published) == 0 {
		return fmt.Errorf("missing published subjects")
	}
	paths, e := filepath.Glob("oci/*/Containerfile")
	if e != nil || len(paths) != len(published) {
		return fmt.Errorf("incomplete publication")
	}
	for _, p := range paths {
		name := filepath.Base(filepath.Dir(p))
		if digest := reused[name]; digest != "" {
			if !regexp.MustCompile(`^sha256:[0-9a-f]{64}$`).MatchString(digest) || digest != published[name] {
				return fmt.Errorf("incorrect reused %s digest", name)
			}
			evidence[name] = digest
			continue
		}
		var r struct {
			Image, Revision, Digest string
			Tested                  bool
		}
		b, e := os.ReadFile("dist/" + name + "/qualified.json")
		if e != nil {
			return e
		}
		if e = json.Unmarshal(b, &r); e != nil {
			return e
		}
		if !r.Tested || r.Image != name || r.Revision != rev || !regexp.MustCompile(`^sha256:[a-f0-9]{64}$`).MatchString(r.Digest) || r.Digest != published[name] {
			return fmt.Errorf("unverified %s manifest", name)
		}
		evidence[name] = r.Digest
	}
	msg, _ := json.Marshal(evidence)
	git := func(args ...string) (string, error) {
		c := exec.Command("git", append([]string{"-c", "safe.directory=*", "-c", "user.name=Release App", "-c", "user.email=release@users.noreply.github.com"}, args...)...)
		c.Stderr = os.Stderr
		b, e := c.Output()
		return strings.TrimSpace(string(b)), e
	}
	head, e := git("rev-parse", "HEAD")
	if e != nil || head != rev {
		return fmt.Errorf("unexpected source")
	}
	remote, e := git("ls-remote", "--exit-code", "origin", "refs/heads/main")
	if e != nil || strings.Fields(remote)[0] != rev {
		return fmt.Errorf("main moved during preparation; qualify the new revision")
	}
	if _, e = git("tag", "-a", version, rev, "-m", string(msg)); e != nil {
		return e
	}
	_, e = git("push", "origin", "refs/tags/"+version)
	return e
}
