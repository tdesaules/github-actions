# Test Catalog

Validates the catalogue directly in Go: independent image ownership, pinned
base images, a Go entrypoint per action and a last stage named `final`.
Prefer scratch for self-contained tools; retain an allowlisted distribution for
complete build environments. Existing unlisted bases/donors produce explicit
migration warnings, not an assertion of policy compliance.
Discovery reads action metadata from the source tree, without a central manifest.
References require an immutable SHA-256 digest by default. The explicit
`allow-source: 'true'` qualification input permits an owned `image: Containerfile`
with a matching final-stage title, or a legacy `:unpublished` reference. Release
tags reject this exception. See [bootstrap removal](../../docs/source-bootstrap.md).

Input `directory` defaults to `.`; `--directory` overrides `INPUT_DIRECTORY`.
`--tool-version` prints the validator version. Sources and rootfs may be read-only.

The image was qualified on AMD64 and ARM64 and published with release `1.0.0`.
The workflow calls this action directly under its changed-file condition.
