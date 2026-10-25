# Test candidate runtime

Validates native architecture, entrypoint, owner and optional source revision,
then tests the candidate executable without starting a nested container engine.
Acquires an OCI layout from an archive or a published immutable reference.
Verifies blob hashes and confines layer extraction, including links and whiteouts.

The candidate is materialized only inside the marked disposable action image.
Bootstrap tools and libraries are removed so they cannot mask missing runtime
dependencies. Tests run in UID/GID 65532 with root-owned read-only system/source
permissions; `/tmp` and `/reports` are explicit writable areas. Runtime mounts
such as `/dev`, `/proc`, DNS and the workspace are preserved. This is permission
enforcement, not a read-only filesystem mount or a general-purpose OCI runtime.

Checks cover valid/invalid/mixed/empty/duplicate/missing literal file lists,
permission failures, genuine suite diagnostics, CLI precedence, paths with spaces,
source immutability, SBOM output permissions, Renovate startup and registry TLS
service lifecycle. Native GitHub action invocations remain separate workflow steps.

Inputs: `archive` or `reference`, `image`, `architecture`, optional `revision`,
`output`. CLI flags override `INPUT_*`. Reports retain individual results and
diagnostics. No arbitrary command input or operation selector is exposed.

The image is awaiting qualification/publication before final workflow activation.
