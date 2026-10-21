# Test Catalog

Validates the catalogue directly in Go: independent image ownership, pinned
base images, a Go entrypoint per action and a last stage of `scratch AS final`.
Discovery reads action metadata from the source tree, without a central manifest.
Explicit `:unpublished` action references allow qualification before publication.
Published references must contain an immutable SHA-256 digest.

Input `directory` defaults to `.`; `--directory` overrides `INPUT_DIRECTORY`.
`--tool-version` prints the validator version. Sources and rootfs may be read-only.

The image was qualified on AMD64 and ARM64 and published with release `1.0.0`.
The workflow calls this action directly under its changed-file condition.
