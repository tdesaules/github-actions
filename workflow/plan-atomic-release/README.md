# Plan Atomic release

**BOOTSTRAP ONLY:** source `image: Containerfile` requires replacement with a
verified native-qualified published action image digest before activation.
Fedora 45 runtime supplies Git and checksum-pinned git-cliff 2.14.2; no engine.

## Inputs and outputs

Inputs: `event`, `ref`, `repository`, `before` (previous push SHA),
`version` (development candidate version, default `0.1.0`), `initial-release`
(boolean, default false), `maintenance` (boolean, default false).

Outputs: `mode`, `source_sha`, `version`, `has_lint`, `has_images`, `has_tests`,
`has_release`, `lint_matrix`, `build_matrix`, `test_matrix`, `plan_artifact`
(`atomic-plan`). The JSON plan is written to `.build/plan.json`.

With `maintenance: true`, only `schedule` or `workflow_dispatch` on
`refs/heads/main` is accepted. The action outputs `mode: maintenance`,
`source_sha`, a `markdown-files` JSON array of tracked Markdown paths, and false
release/build/test/lint flags. It performs no registry access or release planning.
Resolve production registry platforms separately as an advisory maintenance
step so registry failures cannot suppress independent Renovate/link checks.

Use a complete history/tags checkout. Event SHA, when present, must match HEAD.

| Event/ref | Mode | Work |
| --- | --- | --- |
| Branch push | development | Changed source lint/build/test; docs-only skips builds |
| Manual main | prepare | Full release qualification, or explicit no-bump no-op |
| Stable no-v tag push | publish | Full lint and published-digest tests; no builds |
| Other events/manual branches/prerelease tags | rejected | No work |

Preparation computes its own version with fixed git-cliff policy:
`feat` minor, `fix`/`perf` patch, breaking changes major (minor before 1.0).
Other types alone do not release. Without a stable tag, `initial-release: true`
explicitly selects 1.0.0; otherwise preparation is a no-op. This replaces the
prototype's caller-trusted `release-changed` input; do not feed a separate
`workflow/new-release-plan` result into this planner.

Build/test matrices have exactly two OS entries, native `amd64` on
`ubuntu-24.04` and `arm64` on `ubuntu-24.04-arm`. Candidate tests use
`source: candidate`; tag tests use `source: published` and the immutable
multiarch `digest`. No candidate row is emitted for tags.

For tag mode, committed `.github/atomic-release.json` must validate the complete
preparation receipts, immutable package identities, original transport hashes,
native manifest digests and all three ISO payloads. The annotated tag must identify
HEAD, and its metadata commit must be a direct child of the qualified source,
changing only evidence and changelog.

Published test rows additionally contain:

- `iso`, `iso_file`, `iso_sha256`: exact installer artifact identity.
- `iso_files`: JSON array of ISO, checksum and original untested build receipt
  (`path`, `sha256`, `media_type`); use with `oci/get-artifact`.
- `os_archive_sha256`, `os_manifest_digest`: **original AMD64** transport hash
  and manifest digest for ISO qualification, even in the ARM64 row.

Run ISO construction/qualification only on AMD64; preparation constructs it from
the qualified OS archive. `source_sha` in tag mode refers to original image
source, not the final metadata commit. The whole evidence document also appears
under plan `published`, with native records at `qualified.amd64`/`arm64`.

All selected functional tests must pass. Security/SBOM/signature reporting is
explicit and advisory. Honor declared empty phases rather than treating arbitrary
skipped jobs as successful.

`go test ./workflow/plan-atomic-release ./shared/atomicrelease`
