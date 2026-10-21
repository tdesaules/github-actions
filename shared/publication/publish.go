package publication

import (
	"fmt"
	"os"
	"strings"

	"github.com/tdesaules/github-actions/shared/catalog"
	action "github.com/tdesaules/github-actions/shared/runtime"
)

func Publish(directory string) error {
	cleanup, err := Auth()
	if err != nil {
		return err
	}
	defer cleanup()
	state, err := Load(directory)
	if err != nil {
		return err
	}
	for _, image := range state.Images {
		image.Published, image.Signed = false, false
	}
	if err = Save(directory, state); err != nil {
		return err
	}
	owners, err := catalog.Discover()
	if err != nil {
		return err
	}
	for _, owner := range owners {
		image := state.Images[owner.Name]
		if !image.Reused {
			paths, err := ArchivePaths(directory, image)
			if err != nil {
				return err
			}
			layout, err := os.MkdirTemp("", "publish-layout-")
			if err != nil {
				return err
			}
			err = func() error {
				defer os.RemoveAll(layout)
				raw, err := action.AssembleIndex(paths, layout)
				if err != nil {
					return err
				}
				if Hash(raw) != strings.Split(image.Reference, "@")[1] {
					return fmt.Errorf("publication plan differs from archives")
				}
				missing := false
				for _, reference := range References(image) {
					existing, err := rawOptional(reference)
					if err != nil {
						return err
					}
					if existing == nil {
						missing = true
					} else if Hash(existing) != strings.Split(reference, "@")[1] {
						return fmt.Errorf("refusing to overwrite inconsistent registry content: %s", reference)
					}
				}
				if !missing {
					return nil
				}
				return catalog.Run("skopeo", "copy", "--all", "--preserve-digests", "oci:"+layout+":index", "docker://"+image.Reference)
			}()
			if err != nil {
				return err
			}
		}
		raw, err := Raw(image.Reference)
		if err != nil {
			return err
		}
		if Hash(raw) != strings.Split(image.Reference, "@")[1] {
			return fmt.Errorf("published index digest mismatch")
		}
		children, err := Children(raw)
		if err != nil {
			return err
		}
		for arch, digest := range image.Platforms {
			if children[arch] != digest {
				return fmt.Errorf("published child differs from tested archive")
			}
			child, err := Raw(strings.Split(image.Reference, "@")[0] + "@" + digest)
			if err != nil {
				return err
			}
			if Hash(child) != digest {
				return fmt.Errorf("published child digest mismatch")
			}
		}
		image.Published = true
		if err = Save(directory, state); err != nil {
			return err
		}
	}
	return nil
}
