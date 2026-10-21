package publication

import (
	"encoding/base64"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/tdesaules/github-actions/shared/catalog"
)

func Finish(directory string) error {
	state, err := Load(directory)
	if err != nil {
		return err
	}
	var plan ReleasePlan
	if err = catalog.Read(filepath.Join(directory, "release-plan.json"), &plan); err != nil {
		return err
	}
	if !Stable.MatchString(plan.Version) || plan.Version != state.Version || plan.Base != state.Revision {
		return fmt.Errorf("release plan and publication differ")
	}
	for _, image := range state.Images {
		if !image.Signed || !image.Published {
			return fmt.Errorf("all images require verified publication and signatures")
		}
	}
	token := os.Getenv("INPUT_TOKEN")
	if token == "" {
		return fmt.Errorf("release App token is required")
	}
	root, err := os.Getwd()
	if err != nil {
		return err
	}
	env := append(os.Environ(), "GIT_CONFIG_COUNT=2", "GIT_CONFIG_KEY_0=safe.directory", "GIT_CONFIG_VALUE_0="+root, "GIT_CONFIG_KEY_1=http.https://github.com/.extraheader", "GIT_CONFIG_VALUE_1=AUTHORIZATION: basic "+base64.StdEncoding.EncodeToString([]byte("x-access-token:"+token)))
	git := func(args ...string) ([]byte, error) {
		cmd := exec.Command("git", args...)
		cmd.Env = env
		cmd.Stderr = os.Stderr
		return cmd.Output()
	}
	remote, err := git("ls-remote", "origin", "refs/heads/main")
	if err != nil {
		return err
	}
	fields := strings.Fields(string(remote))
	if len(fields) != 2 || fields[0] != plan.Base {
		return fmt.Errorf("main advanced; refusing release push")
	}
	tag, err := git("ls-remote", "origin", "refs/tags/"+plan.Version)
	if err != nil {
		return err
	}
	if len(tag) != 0 {
		return fmt.Errorf("release tag already exists; never move tags")
	}
	status, err := git("status", "--porcelain")
	if err != nil {
		return err
	}
	if len(status) != 0 {
		return fmt.Errorf("release checkout must be clean")
	}
	owners, err := catalog.Discover()
	if err != nil {
		return err
	}
	paths := []string{}
	for _, owner := range owners {
		path := filepath.Join(owner.Directory, "action.yml")
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		updated := catalog.ImageLine.ReplaceAllStringFunc(string(data), func(line string) string {
			match := catalog.ImageLine.FindStringSubmatch(line)
			return strings.Replace(line, match[1], state.Images[owner.Name].Reference, 1)
		})
		if err = os.WriteFile(path, []byte(updated), 0644); err != nil {
			return err
		}
		paths = append(paths, path)
	}
	patch, err := git(append([]string{"diff", "--"}, paths...)...)
	if err != nil {
		return err
	}
	if err = os.WriteFile(filepath.Join(directory, "action-digests.patch"), patch, 0644); err != nil {
		return err
	}
	home, err := os.MkdirTemp("", "release-cliff-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(home)
	config := filepath.Join(home, ".gitconfig")
	if _, err = git("config", "--file", config, "--add", "safe.directory", root); err != nil {
		return err
	}
	cliff := exec.Command("git-cliff", "--config", "cliff.toml", "--tag", plan.Version)
	cliff.Env = append(os.Environ(), "HOME="+home, "XDG_CONFIG_HOME="+home, "GIT_CONFIG_GLOBAL="+config)
	cliff.Stderr = os.Stderr
	notes, err := cliff.Output()
	if err != nil {
		return err
	}
	if err = os.WriteFile("CHANGELOG.md", append([]byte(strings.TrimSpace(string(notes))), '\n'), 0644); err != nil {
		return err
	}
	if _, err = git(append([]string{"add", "CHANGELOG.md"}, paths...)...); err != nil {
		return err
	}
	if _, err = git("-c", "user.name=github-actions[bot]", "-c", "user.email=41898282+github-actions[bot]@users.noreply.github.com", "commit", "-m", "chore(release): "+plan.Version); err != nil {
		return err
	}
	if _, err = git("tag", plan.Version); err != nil {
		return err
	}
	_, err = git("push", "--atomic", "origin", "HEAD:refs/heads/main", "refs/tags/"+plan.Version)
	return err
}
