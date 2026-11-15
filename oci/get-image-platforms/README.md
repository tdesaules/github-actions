# Get immutable image platforms

Read a GHCR multi-platform index once, compute its immutable identity, and return
its AMD64/ARM64 child references. Reject missing/duplicate native platforms and
invalid digests. Non-runnable attestation descriptors are ignored.

Use the returned child references for architecture-specific SBOMs and scans,
including before the first source-bound application release exists. Maintenance
workflows should make registry-resolution errors advisory and still run their
independent dependency updates and link checks.

Skopeo runs inside a Fedora action runtime without a socket. Optional credentials
use stdin and a private temporary auth file. The source Containerfile reference
is bootstrap-only until native qualification and immutable publication complete.
