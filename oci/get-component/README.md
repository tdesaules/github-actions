# Get component

Pulls only a TLS registry reference pinned by SHA-256 using ORAS 1.3.4. A new temporary
sibling directory receives the files; schema, inventory, hashes and contracts
are verified before exposing the requested output. Failed pulls leave no output.

Registry credentials use an isolated 0600 config outside the workspace and are
removed after the operation. Passwords are environment-only. Signature checks
are explicit separate workflow steps; descriptor and registry digests differ.

Pantavisor consumers select GHCR. Local qualification uses an isolated TLS
registry with an ephemeral CA; no plain-HTTP or TLS-verification bypass is exposed.
