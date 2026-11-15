# Publish Atomic release

**BOOTSTRAP ONLY:** `image: Containerfile` must be replaced with the real
native-qualified published action index digest before consumer activation.

Fixed Go Docker action using Fedora 45 for Git/TLS. It performs registry/API
operations directly; no build tool, engine socket or privileged launcher.

## API

Inputs: `version`, `repository`, `qualified-directory` (default `.build/qualified`),
`token` (job-local contents-write token), `registry-username`,
`registry-password` (job-local packages-write token).
Secrets are environment inputs, never command-line arguments.

Output: `release_url`.

Requires `push` at exactly `refs/tags/<version>`, full fetched history/tags and
checkout of that annotated tag. The metadata commit must have the recorded
qualified source as its **only immediate parent** and change only the generated
evidence and changelog. Merely being a descendant is insufficient.

## Fresh tag qualification

The tag workflow is self-contained and skips builds:

1. `workflow/plan-atomic-release` reads committed immutable evidence.
2. `check/test-atomic-image` qualifies the published OS index separately on
   native AMD64/ARM64, with `revision` set to the **qualified source SHA**, not
   the metadata commit. Its receipts must record the published index reference.
3. `oci/get-artifact` downloads the ISO artifact using evidence `iso` and the
   complete `iso_files` array into a new directory.
4. `check/test-atomic-iso` receives the downloaded original untested build receipt,
   expected ISO payload, evidence `qualified.amd64.sha256` (the **original**
   OS archive hash), and `qualified.amd64.manifest_digest`.
5. Transfer both fresh OS receipts/archives and the ISO, checksum, original build
   receipt and fresh ISO qualification into the standard qualified bundle.
6. Invoke this action only after all selected lint and functional tests succeed.

Fresh receipts must match the current tag run (or earlier successful attempts
of that run). OS tar bytes can change on Skopeo re-export: they are hashed against
their fresh receipt, but publication compares native manifest identity against
preparation evidence. The ISO remains bound to the **original** AMD64 archive
checksum. The media tester records current `run_id`/`run_attempt` and preserves
original `build_run_id`/`build_run_attempt`.

## Publication and retries

After all local and published-manifest integrity checks, the remote annotated
tag must still identify the metadata commit. A newer stable remote tag disables
channel promotion and sets `make_latest: false` for this older release.
Serialize release preparation/publication
in one non-cancelling workflow concurrency group.

The exact verified multiarch index bytes are PUT to OS `:45` and `:latest`,
then read back byte-for-byte. These are **OS channels**, never catalog image
aliases. No layers are copied or rebuilt. Registry tag updates cannot be atomic
together; if one update fails, publication fails and a same-tag retry repairs it.

A public, stable GitHub Release is then created with `make_latest: true` only
for the newest stable tag,
the extracted Keep a Changelog version notes, immutable OS/ISO links,
ISO checksum and source/final SHAs. No oversized ISO is attached to GitHub;
the installer remains an ORAS artifact. An identical existing release is an
idempotent success; conflicting release metadata is rejected, not overwritten.
Older maintenance tags still publish their release record without moving
either production channel or replacing GitHub's latest release.

Boot testing is separate local evidence, not fabricated by this action.
Security checks/signing remain advisory explicit workflow steps.

`go test ./shared/atomicrelease ./workflow/publish-atomic-release`
