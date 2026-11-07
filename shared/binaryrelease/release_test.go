package binaryrelease

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestFetchImmutableArtifactSet(t *testing.T) {
	dir, m := qualifiedFixture(t)
	zips := map[int64][]byte{}
	for i := range m.Artifacts {
		binding := &m.Artifacts[i]
		var buf bytes.Buffer
		z := zip.NewWriter(&buf)
		for _, name := range []string{"binary.receipt.json", binding.Archive} {
			b, e := os.ReadFile(filepath.Join(dir, binding.Arch, name))
			if e != nil {
				t.Fatal(e)
			}
			w, e := z.Create(name)
			if e != nil {
				t.Fatal(e)
			}
			if _, e = w.Write(b); e != nil {
				t.Fatal(e)
			}
		}
		if e := z.Close(); e != nil {
			t.Fatal(e)
		}
		zips[binding.ID] = buf.Bytes()
		binding.Digest = fmt.Sprintf("sha256:%x", sha256.Sum256(buf.Bytes()))
	}
	corrupt := false
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/o/r/actions/runs/123":
			_ = json.NewEncoder(w).Encode(Run{ID: 123, HeadSHA: m.SourceSHA, HeadBranch: "main", Event: "workflow_dispatch", Status: "completed", Conclusion: "success", Attempt: 2})
		case "/repos/o/r/actions/runs/123/artifacts":
			arts := []Artifact{}
			for _, b := range m.Artifacts {
				ar := Artifact{ID: b.ID, Name: "qualified-" + b.Arch + "-123", Digest: b.Digest}
				ar.WorkflowRun.ID = 123
				ar.WorkflowRun.HeadSHA = m.SourceSHA
				arts = append(arts, ar)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"artifacts": arts})
		case "/repos/o/r/actions/artifacts/1/zip":
			if corrupt {
				_, _ = w.Write([]byte("forged"))
			} else {
				_, _ = w.Write(zips[1])
			}
		case "/repos/o/r/actions/artifacts/2/zip":
			_, _ = w.Write(zips[2])
		default:
			t.Errorf("unexpected %s", r.URL)
			w.WriteHeader(500)
		}
	}))
	defer s.Close()
	a := API{s.URL, "o/r", "test", s.Client()}
	if e := a.Fetch(m, filepath.Join(t.TempDir(), "qualified"), true); e != nil {
		t.Fatal(e)
	}
	corrupt = true
	if e := a.Fetch(m, filepath.Join(t.TempDir(), "qualified"), true); e == nil {
		t.Fatal("accepted substituted artifact ZIP")
	}
}

func TestReleaseAndAssetPagination(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count := 100
		if r.URL.Query().Get("page") == "2" {
			count = 1
		}
		if strings.Contains(r.URL.Path, "assets") {
			batch := make([]Asset, count)
			_ = json.NewEncoder(w).Encode(batch)
			return
		}
		batch := make([]Release, count)
		for i := range batch {
			batch[i].Tag = "9.9.9"
		}
		if count == 1 {
			batch[0].Tag = "11.0.0"
		}
		_ = json.NewEncoder(w).Encode(batch)
	}))
	defer s.Close()
	a := API{s.URL, "o/r", "test", s.Client()}
	latest, e := a.latest("10.0.0")
	if e != nil || latest {
		t.Fatalf("incorrect latest: %t %v", latest, e)
	}
	assets, e := a.assets(7)
	if e != nil || len(assets) != 101 {
		t.Fatalf("assets pagination: %d %v", len(assets), e)
	}
}

func TestBumpPolicy(t *testing.T) {
	for _, tt := range []struct {
		messages []string
		want     string
	}{{[]string{"docs: clarify"}, ""}, {[]string{"feat(ui): add tab"}, "minor"}, {[]string{"fix: bug"}, "patch"}, {[]string{"perf: cache"}, "patch"}, {[]string{"chore!: incompatible"}, "major"}, {[]string{"docs: migration\n\nBREAKING CHANGE: config removed"}, "major"}, {[]string{"fix: bug", "feat: feature", "chore!: remove"}, "major"}} {
		if got := Bump(tt.messages); got != tt.want {
			t.Errorf("%q: %s != %s", tt.messages, got, tt.want)
		}
	}
}

func TestGitCliffVersionFixtures(t *testing.T) {
	if _, err := exec.LookPath("git-cliff"); err != nil {
		t.Skip("git-cliff is provided by the action image; set PATH for native integration")
	}
	config := "[changelog]\nheader = \"# Changelog\\n\\n## Unreleased\\n\\n\"\nbody = \"{% if version %}## [{{ version }}]{% endif %}\\n{% for commit in commits %}- {{ commit.message }}\\n{% endfor %}\"\n[git]\nconventional_commits = true\nfilter_unconventional = true\nprotect_breaking_commits = true\ntag_pattern = \"^[0-9]+\\\\.[0-9]+\\\\.[0-9]+$\"\ncommit_parsers = [{ breaking = true, group = \"Changed\" }, { message = \"^feat\", group = \"Added\" }, { message = \"^fix\", group = \"Fixed\" }, { message = \"^perf\", group = \"Changed\" }, { message = \".*\", skip = true }]\n[bump]\ninitial_tag = \"1.0.0\"\nfeatures_always_bump_minor = true\nbreaking_always_bump_major = false\n"
	if path := os.Getenv("BINARY_RELEASE_CLIFF_CONFIG"); path != "" {
		b, e := os.ReadFile(path)
		if e != nil {
			t.Fatal(e)
		}
		config = string(b)
	}
	for _, tt := range []struct {
		base, message, want string
		release             bool
	}{
		{"1.2.3", "feat: add tabs", "1.3.0", true}, {"1.2.3", "fix: repair tabs", "1.2.4", true}, {"1.2.3", "perf: cache icons", "1.2.4", true}, {"1.2.3", "chore!: remove config", "2.0.0", true}, {"1.2.3", "docs: migration\n\nBREAKING CHANGE: remove config", "2.0.0", true}, {"1.2.3", "docs: explain tabs", "1.2.3", false}, {"0.2.3", "chore!: remove config", "0.3.0", true},
		{"1.2.3", "FEAT: add tabs", "1.3.0", true}, {"1.2.3", "PERF: cache", "1.2.4", true}, {"1.2.3", "docs: migration\n\nBreaking Change: removed", "2.0.0", true},
	} {
		t.Run(tt.message+tt.base, func(t *testing.T) {
			fixtureRepo(t)
			write(t, "cliff.toml", config)
			commit(t, "chore: configure notes")
			if _, e := Git("tag", tt.base); e != nil {
				t.Fatal(e)
			}
			write(t, "docs/change.md", tt.message)
			commit(t, tt.message)
			got, release, e := releaseVersion(false, "cliff.toml")
			if e != nil || got != tt.want || release != tt.release {
				t.Fatalf("got %s %t %v; want %s %t", got, release, e, tt.want, tt.release)
			}
		})
	}
}

func TestAnnotatedTagSourceBinding(t *testing.T) {
	source := fixtureRepo(t)
	for _, f := range []string{"Cargo.toml", "Cargo.lock"} {
		if e := updateVersion(f, "0.1.0", "1.0.0", f == "Cargo.lock"); e != nil {
			t.Fatal(e)
		}
	}
	write(t, "CHANGELOG.md", "# Changelog\n\n## Unreleased\n\n## [1.0.0] - 2026-11-07\n")
	final := commit(t, "chore(release): prepare 1.0.0")
	_, m := qualifiedFixture(t)
	m.SourceSHA = source
	b, _ := json.Marshal(m)
	if _, e := Git("tag", "-a", "1.0.0", "-m", markerPrefix+string(b)); e != nil {
		t.Fatal(e)
	}
	got, head, e := ReadTag("1.0.0")
	if e != nil || got.SourceSHA != source || head != final {
		t.Fatalf("tag: %+v %s %v", got, head, e)
	}
	p, e := MakePlan(PlanOptions{Event: "push", Ref: "refs/tags/1.0.0", SourceSHA: final})
	if e != nil || p.SourceSHA != source || p.HasImages || !p.HasTests || !p.HasRelease {
		t.Fatalf("tag plan: %+v %v", p, e)
	}
	for _, row := range p.TestMatrix.Include {
		if row["source"] != "published" || row["runtime_test"] != true || row["scan"] != true {
			t.Fatalf("invalid published test row: %#v", row)
		}
	}
	if _, e = Git("tag", "1.1.0"); e != nil {
		t.Fatal(e)
	}
	if _, _, e = ReadTag("1.1.0"); e == nil {
		t.Fatal("accepted lightweight tag")
	}
}
func fixtureRepo(t *testing.T) string {
	t.Helper()
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Chdir(t.TempDir())
	run := func(args ...string) {
		t.Helper()
		if b, e := exec.Command("git", args...).CombinedOutput(); e != nil {
			t.Fatalf("git %v: %s %v", args, b, e)
		}
	}
	run("init", "-b", "main")
	run("config", "user.name", "Fixture")
	run("config", "user.email", "fixture@example.invalid")
	write(t, "Cargo.toml", "[package]\nname = \"niri-tonic-files\"\nversion = \"0.1.0\"\n")
	write(t, "Cargo.lock", "version = 4\n\n[[package]]\nname = \"niri-tonic-files\"\nversion = \"0.1.0\"\n")
	write(t, "src/main.rs", "fn main() {}\n")
	run("add", ".")
	run("commit", "-m", "chore: initial")
	h, _ := Git("rev-parse", "HEAD")
	return h
}
func write(t *testing.T, path, data string) {
	t.Helper()
	if e := os.MkdirAll(filepath.Dir(path), 0755); e != nil {
		t.Fatal(e)
	}
	if e := os.WriteFile(path, []byte(data), 0644); e != nil {
		t.Fatal(e)
	}
}
func commit(t *testing.T, message string) string {
	t.Helper()
	if _, e := Git("add", "-A"); e != nil {
		t.Fatal(e)
	}
	if _, e := Git("commit", "-m", message); e != nil {
		t.Fatal(e)
	}
	h, _ := Git("rev-parse", "HEAD")
	return h
}
func TestPlannerRoutingAndDiffs(t *testing.T) {
	source := fixtureRepo(t)
	o := PlanOptions{Event: "push", Ref: "refs/heads/main", SourceSHA: source, Before: strings.Repeat("0", 40)}
	p, e := MakePlan(o)
	if e != nil || !p.HasImages || len(p.BuildMatrix.Include) != 2 {
		t.Fatalf("new branch: %+v %v", p, e)
	}
	o.Before = source
	p, e = MakePlan(o)
	if e != nil || p.HasLint || p.HasImages {
		t.Fatalf("empty: %+v %v", p, e)
	}
	write(t, "docs/space name.md", "documentation")
	o.SourceSHA = commit(t, "docs: guide")
	p, e = MakePlan(o)
	if e != nil || !p.HasLint || p.HasImages {
		t.Fatalf("docs: %+v %v", p, e)
	}
	o.Before = o.SourceSHA
	if e = os.Rename("src/main.rs", "docs/renamed.rs"); e != nil {
		t.Fatal(e)
	}
	o.SourceSHA = commit(t, "refactor: move")
	p, e = MakePlan(o)
	if e != nil || !p.HasImages {
		t.Fatalf("rename/deletion: %+v %v", p, e)
	}
	o.Event = "workflow_dispatch"
	o.Mode = "prepare"
	p, e = MakePlan(o)
	if e != nil || p.HasRelease || p.Version != "0.1.0" {
		t.Fatalf("initial development: %+v %v", p, e)
	}
	o.Initial = true
	p, e = MakePlan(o)
	if e != nil || !p.HasRelease || p.Version != "1.0.0" {
		t.Fatalf("initial official: %+v %v", p, e)
	}
	o.Ref = "refs/heads/other"
	if _, e = MakePlan(o); e == nil {
		t.Fatal("accepted wrong prepare ref")
	}
	o.Event = "push"
	o.Ref = "refs/tags/v1.0.0"
	if _, e = MakePlan(o); e == nil {
		t.Fatal("accepted v tag")
	}
	o.Ref = "refs/tags/1.0.0-rc.1"
	if _, e = MakePlan(o); e == nil {
		t.Fatal("accepted prerelease")
	}
	o.Ref = "refs/tags/1.0.0"
	if _, e = MakePlan(o); e == nil {
		t.Fatal("accepted unknown tag")
	}
}
func TestPrepareNoBumpAndMetadata(t *testing.T) {
	source := fixtureRepo(t)
	if _, e := Git("tag", "1.0.0"); e != nil {
		t.Fatal(e)
	}
	write(t, "docs/guide.md", "docs")
	source = commit(t, "docs: guide")
	p, e := MakePlan(PlanOptions{Event: "workflow_dispatch", Ref: "refs/heads/main", SourceSHA: source, Mode: "prepare"})
	if e != nil || p.HasRelease || p.HasImages || p.Version != "1.0.0" {
		t.Fatalf("no bump: %+v %v", p, e)
	}
	for _, f := range []string{"Cargo.toml", "Cargo.lock"} {
		if e = updateVersion(f, "0.1.0", "1.1.0", f == "Cargo.lock"); e != nil {
			t.Fatal(e)
		}
	}
	write(t, "CHANGELOG.md", "# Changelog\n\n## [Unreleased]\n\n## [1.1.0] - 2026-11-07\n\n### Added\n\n- Tabs.\n")
	final := commit(t, "chore(release): prepare 1.1.0")
	if e = validateMetadata(source, final, "1.1.0"); e != nil {
		t.Fatal(e)
	}
	write(t, "src/main.rs", "fn main() { panic!() }\n")
	if _, e = Git("add", "."); e != nil {
		t.Fatal(e)
	}
	if _, e = Git("commit", "--amend", "--no-edit"); e != nil {
		t.Fatal(e)
	}
	final, _ = Git("rev-parse", "HEAD")
	if e = validateMetadata(source, final, "1.1.0"); e == nil {
		t.Fatal("accepted source modification")
	}
}
func qualifiedFixture(t *testing.T) (string, Marker) {
	t.Helper()
	dir := t.TempDir()
	m := Marker{SchemaVersion: 1, SourceSHA: strings.Repeat("a", 40), Version: "1.0.0", RunID: "123", RunAttempt: "2"}
	for i, arch := range []string{"amd64", "arm64"} {
		name := "niri-tonic-files-1.0.0-linux-" + arch + ".tar.gz"
		path := filepath.Join(dir, arch, name)
		write(t, path, "exact binary "+arch)
		sum, _ := Hash(path)
		r := Receipt{1, m.SourceSHA, m.Version, Component, arch, name, sum, m.RunID, m.RunAttempt, true}
		if e := WriteJSON(filepath.Join(dir, arch, "binary.receipt.json"), r); e != nil {
			t.Fatal(e)
		}
		m.Artifacts = append(m.Artifacts, ArtifactBinding{int64(i + 1), arch, "sha256:" + strings.Repeat("b", 64), sum, name})
	}
	return dir, m
}
func TestQualificationIntegrity(t *testing.T) {
	for _, kind := range []string{"valid", "source", "hash", "untested", "missing", "attempt", "binding"} {
		t.Run(kind, func(t *testing.T) {
			dir, m := qualifiedFixture(t)
			var r Receipt
			path := filepath.Join(dir, "amd64", "binary.receipt.json")
			if e := Decode(path, &r); e != nil {
				t.Fatal(e)
			}
			switch kind {
			case "source":
				r.SourceSHA = strings.Repeat("c", 40)
			case "hash":
				r.SHA256 = strings.Repeat("0", 64)
			case "untested":
				r.Tested = false
			case "attempt":
				r.RunAttempt = "1"
			case "binding":
				m.Artifacts[0].SHA256 = strings.Repeat("0", 64)
			}
			if e := WriteJSON(path, r); e != nil {
				t.Fatal(e)
			}
			if kind == "missing" {
				os.Remove(path)
			}
			q, e := Verify(dir, m.SourceSHA, m.Version, m.RunID, m.RunAttempt)
			if e == nil {
				e = MatchBindings(q, m)
			}
			if (kind == "valid") != (e == nil) {
				t.Fatalf("%s: %v", kind, e)
			}
		})
	}
}
func TestArtifactPaginationAndRunIdentity(t *testing.T) {
	calls := 0
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		items := []Artifact{}
		if r.URL.Query().Get("page") == "1" {
			for i := 0; i < 100; i++ {
				items = append(items, Artifact{ID: int64(i + 1)})
			}
		} else {
			items = append(items, Artifact{ID: 101})
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"artifacts": items})
	}))
	defer s.Close()
	a := API{s.URL, "owner/repo", "test", s.Client()}
	art, e := a.Artifacts("123")
	if e != nil || len(art) != 101 || calls != 2 {
		t.Fatalf("pagination %d %d %v", len(art), calls, e)
	}
	_, m := qualifiedFixture(t)
	r := Run{ID: 123, HeadSHA: m.SourceSHA, HeadBranch: "main", Event: "workflow_dispatch", Status: "in_progress", Attempt: 2}
	if e = checkRun(r, m, false); e != nil {
		t.Fatal(e)
	}
	r.Attempt = 3
	if e = checkRun(r, m, false); e == nil {
		t.Fatal("accepted artifact from older attempt")
	}
}
func TestZipTraversal(t *testing.T) {
	for _, name := range []string{"../escape.json", "/absolute.json", "a\\evil.json"} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "bad.zip")
			f, e := os.Create(path)
			if e != nil {
				t.Fatal(e)
			}
			z := zip.NewWriter(f)
			entry, _ := z.Create(name)
			_, _ = entry.Write([]byte("{}"))
			_ = z.Close()
			_ = f.Close()
			if e = extract(path, t.TempDir()); e == nil {
				t.Fatal("accepted unsafe zip")
			}
		})
	}
}
func TestPublicationIdempotencyAndConflicts(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "binary.tar.gz")
	write(t, path, "exact bytes")
	sum, _ := Hash(path)
	uploads := 0
	exists := false
	conflict := false
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/repos/o/r/releases" && r.Method == "GET":
			fmt.Fprint(w, `[]`)
		case r.URL.Path == "/repos/o/r/releases/tags/1.0.0":
			if !exists {
				w.WriteHeader(404)
				return
			}
			fmt.Fprint(w, `{"id":7,"tag_name":"1.0.0"}`)
		case r.URL.Path == "/repos/o/r/releases" && r.Method == "POST":
			exists = true
			w.WriteHeader(201)
			fmt.Fprint(w, `{"id":7,"tag_name":"1.0.0"}`)
		case r.URL.Path == "/repos/o/r/releases/7" && r.Method == "PATCH":
			fmt.Fprint(w, `{"id":7,"tag_name":"1.0.0"}`)
		case r.URL.Path == "/repos/o/r/releases/7/assets" && r.Method == "GET":
			if uploads == 0 {
				fmt.Fprint(w, `[]`)
			} else {
				digest := "sha256:" + sum
				if conflict {
					digest = "sha256:" + strings.Repeat("0", 64)
				}
				_ = json.NewEncoder(w).Encode([]Asset{{ID: 9, Name: "binary.tar.gz", Size: 11, Digest: digest}})
			}
		case r.URL.Path == "/repos/o/r/releases/7/assets" && r.Method == "POST":
			uploads++
			w.WriteHeader(201)
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL)
			w.WriteHeader(500)
		}
	}))
	defer s.Close()
	a := API{s.URL, "o/r", "test", s.Client()}
	for i := 0; i < 2; i++ {
		if e := a.Publish("1.0.0", "notes", []string{path}); e != nil {
			t.Fatal(e)
		}
	}
	if uploads != 1 {
		t.Fatalf("uploaded %d times", uploads)
	}
	conflict = true
	if e := a.Publish("1.0.0", "notes", []string{path}); e == nil {
		t.Fatal("replaced conflicting asset")
	}
}
