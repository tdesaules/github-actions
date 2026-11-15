# Prepare Atomic release

**BOOTSTRAP ONLY:** source metadata uses `image: Containerfile`. Consumers must
wait for actual AMD64/ARM64 hosted qualification, publication and digest pinning.
No image digest is implied by local Go tests.

Fixed Go Docker action with Fedora 45, Git and checksum-pinned git-cliff 2.14.2.
No container engine, socket or privileged runtime is used.

## API

Required inputs: `source-sha`, `repository`, `image`, `iso`, `token`,
`registry-username`, `registry-password`.

- `image`: exact immutable `reference`/image output of `oci/merge-image-index`,
  at `ghcr.io/<repository>@sha256:...`.
- `iso`: exact immutable `reference` output of `oci/publish-artifact`,
  at `ghcr.io/<repository>-iso@sha256:...`.
- The ISO publication array is read from `iso-qualification.json`'s persisted
  `files` field and revalidated. No inline cross-job payload-list input is needed.
- `token`: job-local, repository-authorized App installation token generated
  by the official commit-pinned App-token action using `vars.BOT_APP_ID` and
  `secrets.BOT_APP_PRIVATE_KEY`. No PAT or reused job token.
- Registry credentials: a job-local token with packages read access to verify
  the already-published immutable manifests. Tokens are passed only via action
  environment; Git uses a private temporary askpass file, not token arguments.

Optional inputs: `qualified-directory` (`.build/qualified`), `plan`
(`.build/plan.json`), `config` (`cliff.toml`).

Outputs: `changed`, `tag`, `source_sha`, `final_sha`, `image`, `iso`.
`changed: false` is a successful no-bump operation, with no remote mutation.

## Execution

Requires full fetched Git history/tags, clean tracked checkout/index at the captured
source, and `workflow_dispatch` on `refs/heads/main`. The planner supplies
`schema_version`, `mode`, `source_sha`, `version`, `has_release`, `initial_release`.
Preparation independently recomputes the release policy; it cannot accept an
arbitrary caller-selected version.

The operation:

1. Rehashes the complete current-run qualified bundle and ISO publication files.
2. Validates the App installation token's repository access.
3. Downloads the exact published index and ISO manifest. Checks both native
   manifest digests and the complete three-file ISO checksum/media-type set.
4. Rejects main advancement or any existing destination tag.
5. Generates notes with `git-cliff --offline --no-exec`; requires dated Keep a
   Changelog version and Unreleased headings plus the version comparison link.
6. Commits only `CHANGELOG.md` and `.github/atomic-release.json`, as a direct
   child of the qualified source; rechecks remote refs.
7. Creates an annotated no-v stable tag, then pushes main and tag together with
   `--atomic` and an exact main lease. Tag collisions never force-update tags.

Configure git-cliff notes with nonempty Keep a Changelog sections and comparison
links. Bump calculation uses a fixed separate configuration so note filtering
cannot hide breaking changes. `feat` means minor; `fix`/`perf` patch; breaking
changes major (minor before 1.0). Other changes alone are a no-op. The first
official release requires planner `initial-release: true` and is exactly 1.0.0.

Generated evidence records source SHA, version, OS index/ISO immutable references,
`iso_file`, `iso_sha256`, `iso_files`, architectures `[amd64, arm64]`, and complete
`qualified.amd64`, `qualified.arm64`, `qualified.iso` receipts. Native manifest
digests and original archive checksums remain separately available. The final
metadata SHA is the tagged commit, not the source compiled into the image.

Publish exact qualified artifacts first, then run this action. Keep signing and
security-reporting steps explicit and advisory. Serialize preparation and
publication with a shared non-cancelling release concurrency group.

## Retry behavior

Failures leave local evidence for diagnosis. Rerun from a fresh captured-source
checkout; a published main/tag pair is not moved or recreated. Immutable registry
objects from an interrupted preparation may remain unreferenced. Once the atomic
push succeeds, the App-generated tag event enters the self-contained release
workflow; it does not rebuild.

`go test ./shared/atomicrelease ./workflow/prepare-atomic-release`
