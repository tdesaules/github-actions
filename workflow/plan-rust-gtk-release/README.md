# Plan Rust GTK release

Dedicated Docker action for `niri-tonic-files`. Reads the consumer checkout and
git history, never a catalog manifest. Use full-history checkout including tags.

Inputs: `directory` (default `.`), `event`, `ref`, `before`, `source-sha`, `mode`, `initial-release` (default
`false`), `output` (default `.build/plan.json`), `config` (default `cliff.toml`).
`source-sha` must equal HEAD. Dispatch preparation requires `refs/heads/main` and
`mode: prepare`; maintenance dispatch/schedules emit an empty maintenance plan.
Git, Cargo and relative `config` paths use `directory`; relative `output` paths use
the original workspace, even when the application checkout path contains spaces.
All inputs have named CLI equivalents (`--source-sha=...`, `--directory ...`).
Explicit CLI values, including empty strings, override environment/default values.
Unknown or positional arguments fail without echoing values. `--version` and
`--tool-version` print the action version without performing work.

Outputs: `mode`, `source_sha`, `version`, `has_lint`, `has_images`, `has_tests`,
`has_release`, `lint_matrix`, `build_matrix`, `test_matrix`, `plan_artifact`.
Matrices are JSON `{"include": [...]}` objects, including empty arrays. Lint rows
contain `id`, `action_path`, `runner`, and `files` (literal existing tracked paths).
IDs are `rust-gtk`, `actionlint`, `yaml`, `toml`, and `markdownlint`, mapped to
`lint/<id>`. Call these actions through literal `uses` steps selected by `matrix.id`.
Build/test entries contain `component`, `directory: "."`, `arch`
(`amd64`/`arm64`), and native Ubuntu `runner`. Test rows additionally contain
`source: candidate|published` and boolean `runtime_test: true`, `scan: true`.
Binary test rows have no image digest. All action inputs have explicit `INPUT_*`
environment mappings with underscores in their runtime names.
`has_images` selects binary construction for the common six-job contract.

Development uses NUL-delimited changed filenames, detects both sides of renames,
and includes deletions. An explicit all-zero new-branch SHA selects all tracked
source. Invalid or unavailable push-base history fails and requires fetching that
commit; it never substitutes an unrelated merge base. Docs-only changes select
Markdown checks without Rust/Clippy or binary construction. Deleted files trigger
validation of remaining files of the applicable kind, never nonexistent file paths.
Markdown configuration changes select all Markdown. Runtime/plugin/workflow changes
and `cliff.toml` changes conservatively select binary build/tests. The latter also
selects TOML validation. Preparation and publication select all applicable lints;
actionlint is absent when no workflows exist. Preparation qualifies both architectures;
no-bump preparation emits empty matrices and `has_release: false`.

The first release is explicitly `initial-release: true` → `1.0.0`; otherwise
initial development remains `0.1.0`. After release, full commit messages determine
the highest explicit git-cliff bump, with case-insensitive matching: feat/minor,
fix or perf/patch, any breaking
type/major (minor before 1.0). Note filtering cannot suppress a bump. Other commits
alone do not release. Version queries use a temporary unfiltered copy of the app
configuration so an entirely hidden commit range still receives the explicit bump;
release-note generation uses the original configuration. Tags must be stable no-v
annotated preparation tags. Tag
plans skip build and return the original build `source_sha`, after proving the
final tag commit changes only Cargo versions and changelog metadata.

## Runtime and verification

Fedora runtime retains git and checksum-verified git-cliff 2.14.2. The recipe
reuses the catalog's pinned Fedora/Go/cliff versions and verified release hashes.
`image: Containerfile` is temporary source bootstrap metadata: replace with the
qualified GHCR digest after publication. No placeholder digest is supplied.

Tests: `go test ./shared/binaryrelease`. For actual git-cliff fixtures, put 2.14.2
on PATH and set `BINARY_RELEASE_CLIFF_CONFIG` to the consumer's absolute config
path. Network tests use local HTTP servers only.
