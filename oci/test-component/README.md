# Test component

Verifies a public Pantavisor schema-2 ARM64 component without importing any
producer code. Input `directory` selects an existing export. CLI input overrides
`INPUT_DIRECTORY`. No network access or source writes are performed.

Checks the exact inventory, regular files only, SHA-256 and sizes, required
payloads, source identity, build provenance and runtime/boot contract consistency.
Private rendered states and arbitrary OCI image layers are not components.

Outputs: `name`, `version`, `kind` and `digest`. The latter hashes the descriptor
bytes; it is **not** the OCI/ORAS manifest digest. A development component with
`source.dirty: true` may be verified; release policy must reject it separately.
Integrity checks do not authenticate the publisher or replace signature checks.

The `:unpublished` bootstrap reference must be replaced by the qualified published
image digest before this action is consumed through a release tag.
