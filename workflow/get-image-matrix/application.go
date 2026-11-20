package main

import (
	"encoding/json"
	"fmt"
	"github.com/tdesaules/github-actions/shared/workflow"
	"os"
	"regexp"
	"strings"
)

var stableAppTag = regexp.MustCompile(`^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)$`)
var appDigest = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)

func applicationPlan(definition, repo, event, ref string, paths []string, full bool) error {
	tag := strings.HasPrefix(ref, "refs/tags/")
	maintenance := event == "maintenance" || event == "schedule"
	prepare := event == "workflow_dispatch"
	if (maintenance || prepare) && ref != "refs/heads/main" {
		return fmt.Errorf("manual and maintenance runs require main")
	}
	if tag && !stableAppTag.MatchString(strings.TrimPrefix(ref, "refs/tags/")) {
		return fmt.Errorf("invalid application release tag")
	}
	tracked, e := workflow.Git("ls-files", "-z")
	if e != nil {
		return e
	}
	all := []string{}
	for _, p := range workflow.SplitPaths(tracked) {
		s := strings.Split(p, "/")
		if len(s) == 3 && s[0] == "oci" && s[2] == "Containerfile" {
			if !regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`).MatchString(s[1]) {
				return fmt.Errorf("invalid component %s", p)
			}
			all = append(all, s[1])
		}
	}
	if len(all) == 0 {
		return fmt.Errorf("no application Containerfiles")
	}
	previous := ""
	if tag {
		previous = ref
	} else if maintenance || prepare {
		tags, e := workflow.Git("tag", "--merged", "HEAD", "--sort=-version:refname")
		if e != nil {
			return e
		}
		for _, t := range strings.Fields(tags) {
			if stableAppTag.MatchString(t) {
				previous = "refs/tags/" + t
				break
			}
		}
	}
	digests := map[string]string{}
	if previous != "" {
		s, e := workflow.Git("for-each-ref", "--format=%(contents:subject)", previous)
		if e != nil {
			return e
		}
		if e = json.Unmarshal([]byte(s), &digests); e != nil {
			return fmt.Errorf("release lacks application digests: %w", e)
		}
	} else {
		var d struct{ Images map[string]string }
		b, e := os.ReadFile(definition)
		if e != nil {
			return e
		}
		if e = json.Unmarshal(b, &d); e != nil {
			return e
		}
		for _, n := range all {
			v, ok := strings.CutPrefix(d.Images[n], "ghcr.io/"+repo+"/"+n+"@")
			if ok {
				digests[n] = v
			}
		}
	}
	if prepare && previous != "" {
		paths, e = workflow.Paths(previous, "HEAD")
		if e != nil {
			return e
		}
		full = false
	}
	images, builds, scans := []string{}, []string{}, []string{}
	reused := map[string]string{}
	platform := full || prepare || tag
	for _, f := range paths {
		if !strings.HasSuffix(f, ".md") && !strings.HasPrefix(f, "docs/") {
			platform = true
		}
	}
	for _, n := range all {
		build := full || prepare && previous == ""
		test := prepare || tag
		scan := false
		for _, f := range paths {
			build = build || strings.HasPrefix(f, "oci/"+n+"/") || f == "oci/prepare.sh" || f == ".dockerignore"
			test = test || f == ".github/scripts/smoke-"+n+".py" || strings.HasPrefix(f, ".github/workflows/") || f == definition
			scan = scan || strings.HasPrefix(f, ".grype") || strings.HasPrefix(f, ".syft")
		}
		build = build && !tag && !maintenance
		if !build && (test || scan || maintenance) && !appDigest.MatchString(digests[n]) {
			return fmt.Errorf("missing immutable %s subject", n)
		}
		if maintenance {
			continue
		}
		if build || test || scan {
			images = append(images, n)
			if build {
				builds = append(builds, n)
			} else {
				reused[n] = digests[n]
				if scan && !test {
					scans = append(scans, n)
				}
			}
		}
	}
	for k, v := range map[string]any{"images": images, "build_images": builds, "scan_only": scans, "has_tests": len(images) > 0, "has_build": len(builds) > 0, "platform": platform, "reused": reused, "digests": digests} {
		if e := workflow.Output(k, v); e != nil {
			return e
		}
	}
	return nil
}
