package publication

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/tdesaules/github-actions/shared/catalog"
)

// Aggregate verifies the exact catalogue set after independently published image
// chains. A receipt never authorizes a Git write without remote digest checks.
func Aggregate(directory, receipts, planPath string) error {
	var plan ReleasePlan
	if err := catalog.Read(planPath, &plan); err != nil {
		return err
	}
	revision, err := Revision()
	if err != nil {
		return err
	}
	if plan.Base != revision || !plan.Changed || !Stable.MatchString(plan.Version) {
		return fmt.Errorf("invalid aggregate release plan")
	}
	expected, err := expectedBuilds(planPath, "")
	if err != nil {
		return err
	}
	state := State{RunID: os.Getenv("GITHUB_RUN_ID"), Revision: revision, Version: plan.Version, Images: map[string]*Image{}}
	err = filepath.WalkDir(receipts, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("symlink in publication receipts")
		}
		if entry.IsDir() || entry.Name() != "publication.json" {
			return nil
		}
		part, err := Load(filepath.Dir(path))
		if err != nil {
			return err
		}
		if part.Image == "" || part.Version != plan.Version || len(part.Images) != 1 || state.Images[part.Image] != nil {
			return fmt.Errorf("unscoped, duplicate or stale image receipt")
		}
		image := part.Images[part.Image]
		if image == nil || !image.Published {
			return fmt.Errorf("image publication incomplete")
		}
		for _, arch := range []string{"amd64", "arm64"} {
			if expected[part.Image+"/"+arch] {
				archive := image.Archives[arch]
				if image.Reused || archive.Path == "" || !Digest.MatchString("sha256:"+archive.SHA256) {
					return fmt.Errorf("rebuilt image lacks qualified archive evidence")
				}
			} else if !image.Reused {
				return fmt.Errorf("unplanned image rebuild")
			}
		}
		raw, err := Raw(image.Reference)
		if err != nil {
			return err
		}
		if Hash(raw) != strings.Split(image.Reference, "@")[1] {
			return fmt.Errorf("published index digest mismatch")
		}
		platforms, err := Children(raw)
		if err != nil {
			return err
		}
		for _, arch := range []string{"amd64", "arm64"} {
			if platforms[arch] != image.Platforms[arch] {
				return fmt.Errorf("published platform differs from receipt")
			}
			child, err := Raw(strings.Split(image.Reference, "@")[0] + "@" + platforms[arch])
			if err != nil {
				return err
			}
			if Hash(child) != platforms[arch] {
				return fmt.Errorf("published platform digest mismatch")
			}
		}
		state.Images[part.Image] = image
		return nil
	})
	if err != nil {
		return err
	}
	if err := Save(directory, state); err != nil {
		return err
	}
	_, err = Load(directory) // Complete ownership and unchanged-digest validation.
	return err
}
