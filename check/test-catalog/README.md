# Test Catalog

Validates the catalogue directly in Go: independent image ownership, pinned
base images, a Go entrypoint per action and a last stage of `scratch AS final`.
Discovery reads action metadata from the source tree, without a central manifest.
Explicit `:unpublished` action references allow qualification before publication.
Published references must contain an immutable SHA-256 digest.

Input `directory` defaults to `.`; `--directory` overrides `INPUT_DIRECTORY`.
`--tool-version` prints the validator version. Sources and rootfs may be read-only.

The image is awaiting native qualification and immutable publication. The main
workflow retains an explicitly named bootstrap step until publication completes.
