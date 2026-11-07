# Qualify Rust/GTK exact candidate

Inputs: `directory` (`.`), required `archive`, `receipt`, `source-sha`, `version`,
`arch`, `source` (`candidate`), optional `source-run-id`/`source-run-attempt`,
and `output` (`.build/qualified`). Paths resolve relative to the initial
workspace. Outputs: `archive`, `receipt`, `sha256`, `extracted-root` (workspace paths).

Checks native CPU, full HEAD/source/version/component identity, archive basename
and checksum. Tracked worktree/index changes are rejected before and after functional
checks; generated untracked .build outputs do not invalidate source identity.
Candidate mode requires tested:false and both current GITHUB_RUN_ID
and GITHUB_RUN_ATTEMPT; source-run overrides are rejected. Published mode requires
tested:true and explicit source-run-id/source-run-attempt from read-binary-release,
matching the original preparation receipt. It does not substitute the tag run's
identity. Both modes execute all functional checks. Extracts only
regular files with safe relative paths into a fresh directory; rejects links,
duplicate entries and oversized archives. Runs full `cargo test --locked -j2`,
Filen `npm ci --ignore-scripts` and `npm test`, Google Drive `npm test`, and
`python3 scripts/check-connectors.py --protocol-only`. If `.github/scripts` exists,
also runs `python3 -m unittest discover -s .github/scripts -p test_*.py`; the image
includes packaged Python PyYAML for workflow policy tests. Uses isolated HOME/XDG
paths, with no real accounts. Source tests never substitute their binary for the
candidate. The exact extracted binary must report `niri-tonic-files <version>`,
pass `--check-config` and map its `--smoke-test` UI on private headless Weston
with software rendering and a private D-Bus session. No host sockets or privileges.

Rechecks the input checksum, copies exact archive bytes, verifies the copy, then
writes a tested:true receipt preserving the original run ID/attempt in both modes.
The receipt filename remains `<archive>.receipt.json`. A fresh, persistent extraction
of the verified output archive is exposed as extracted-root for Syft payload scans;
source files and test-created files are not included. Failures produce
output/test-report.json. This separate report records the current verification run
ID/attempt without modifying receipt provenance. Outputs
are mode 0644 and directories 0755; Docker adopts a non-root checkout owner's UID.
The runtime retains Fedora 44 Rust/GTK, Node/npm, Python, Weston, D-Bus, Mesa and
fonts because qualification needs their real runtime closure. Static Go uses the
catalog's pinned Fedora 45 builder. CLI flags override INPUT_* values.

Bootstrap metadata uses Containerfile until actual qualified GHCR publication.
Native hosted qualification is required on ubuntu-24.04 and ubuntu-24.04-arm.
Catalog `test-candidate-runtime` needs explicit cases for the three new image
IDs; use the fixture under shared/rustgtk/testdata/application for image-level
qualification, followed by actual consumer integration. Tests:
`go test ./workflow/test-rust-gtk ./shared/rustgtk`.
