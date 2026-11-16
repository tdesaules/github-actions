package main

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestReleaseMetadataCommitRequiresExactPinOnlyTaggedCommit(t *testing.T) {
	for _, scenario := range []string{"pins", "source-promotion", "interface", "source", "untagged", "wrong-tag", "wrong-subject", "range"} {
		t.Run(scenario, func(t *testing.T) {
			t.Chdir(t.TempDir())
			git := func(args ...string) string {
				t.Helper()
				data, err := exec.Command("git", args...).CombinedOutput()
				if err != nil {
					t.Fatalf("git %v: %s %v", args, data, err)
				}
				return strings.TrimSpace(string(data))
			}
			write := func(path, text string) {
				t.Helper()
				if err := os.WriteFile(path, []byte(text), 0644); err != nil {
					t.Fatal(err)
				}
			}
			git("init", "-b", "main")
			git("config", "user.name", "Fixture")
			git("config", "user.email", "fixture@example.invalid")
			git("config", "commit.gpgsign", "false")
			git("config", "tag.gpgsign", "false")
			if err := os.MkdirAll("check/tool", 0755); err != nil {
				t.Fatal(err)
			}
			prefix := "ghcr.io/tdesaules/github-actions/check-tool@sha256:"
			old := "name: tool\nruns:\n  using: docker\n  image: docker://" + prefix + strings.Repeat("a", 64) + "\n"
			if scenario == "source-promotion" {
				old = "name: tool\nruns:\n  using: docker\n  image: Containerfile\n"
			}
			write("check/tool/action.yml", old)
			write("check/tool/Containerfile", "FROM scratch AS final\nLABEL org.opencontainers.image.title=\"check-tool\"\n")
			git("add", ".")
			git("commit", "-m", "feat: tool")
			before := git("rev-parse", "HEAD")
			if scenario == "range" {
				write("extra", "another change")
				git("add", ".")
				git("commit", "-m", "fix: other change")
			}
			next := "name: tool\nruns:\n  using: docker\n  image: docker://" + prefix + strings.Repeat("b", 64) + "\n"
			if scenario == "interface" {
				next += "  args: [--new-behavior]\n"
			}
			write("check/tool/action.yml", next)
			write("CHANGELOG.md", "# Changelog\n")
			if scenario == "source" {
				write("check/tool/main.go", "package main\n")
			}
			subject := "chore(release): 1.2.3"
			if scenario == "wrong-subject" {
				subject = "fix: change action"
			}
			git("add", ".")
			git("commit", "-m", subject)
			head := git("rev-parse", "HEAD")
			if scenario != "untagged" {
				target := head
				if scenario == "wrong-tag" {
					target = before
				}
				git("tag", "1.2.3", target)
			}
			components := []component{{"check/tool", "check-tool", prefix + strings.Repeat("b", 64)}}
			want := scenario == "pins" || scenario == "source-promotion"
			if got := releaseMetadataCommit(before, head, components); got != want {
				t.Fatalf("got %t, want %t", got, want)
			}
			if want {
				outputs, err := invokePlan(t, "--event=push", "--ref=refs/heads/main", "--before="+before, `--files=["check/tool/action.yml","CHANGELOG.md"]`)
				if err != nil || outputs["has-work"] != "false" {
					t.Fatalf("release pin promotion started image chains: %v %v", outputs, err)
				}
				outputs, err = invokePlan(t, "--event=push", "--ref=refs/tags/1.2.3")
				if err != nil || outputs["has-work"] != "true" || outputs["has-build"] != "false" {
					t.Fatalf("tag qualification was lost: %v %v", outputs, err)
				}
			}
		})
	}
}
