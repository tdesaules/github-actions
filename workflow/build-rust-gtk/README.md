# Build Rust/GTK candidate

Builds the native `niri-tonic-files` release binary, without tests. Inputs:
`directory` (`.`), required `version` (stable SemVer, including development
`0.1.0`), required `source-sha` (full lowercase HEAD SHA), required `arch`
(`amd64` or `arm64`, must match CPU), `output` (`.build/candidate`).
`GITHUB_RUN_ID` and `GITHUB_RUN_ATTEMPT` are required.

Outputs: `archive`, `receipt`, `sha256`. Paths are workspace-relative when
GITHUB_WORKSPACE is set. All input paths resolve before changing directory.
Cargo uses `--release --locked -j2`, a clean temporary target directory, and
`NIRI_TONIC_RELEASE_VERSION`. Fedora 44 packaged Rust/GTK preserve the ABI
baseline; no rustup download or cross-architecture execution is used.

The sorted timestamp-normalized tar.gz contains the binary, desktop entry,
scalable icon, application LICENSE, Markdown docs, README, original Cargo.lock, example configuration,
installation instructions, and bounded Filen/Google Drive plugin sources, lockfiles
and licenses. It excludes user configuration, credentials, node_modules, tests,
target and .git. Resources are selected from committed Git tree entries and read
from their immutable blobs; untracked public-looking files are never included.
Tracked worktree and index changes are rejected before/after building and before
receipting. Archive timestamps derive from the source commit. The receipt
is outside the archive, schema_version 1, tested false, with source, version,
component, arch, archive basename, SHA256 and run identity.

Plugin runtimes require external Node >=22. Filen requires
`npm ci --omit=dev --ignore-scripts` in the installed plugin directory. Plugins are
automatically discovered when bin/share use the same prefix. Cargo.lock retains
the source package version; the compiled override is reported by `--version`.

The static Go executable uses the pinned Fedora 45 builder; the runtime needs
Fedora 44 compiler/GTK headers and cannot be scratch. Docker starts as root then
adopts a non-root checkout owner's UID/GID. CLI flags override INPUT_* values.
Bootstrap `image: Containerfile` must be replaced after real native qualification
and GHCR publication. Tests: `go test ./workflow/build-rust-gtk ./shared/rustgtk`.
