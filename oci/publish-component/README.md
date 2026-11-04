# Publish component

Requires a verified schema-2 export with clean source provenance. ORAS 1.3.4
stages a deterministic OCI artifact, rechecks the source export, then copies it
to a TLS registry by digest and verifies the remote digest. Version tags are not overwritten;
the GitHub Release manifest provides the version-to-digest mapping.

Returns the registry `reference` and separate `descriptor-digest`. Authentication
uses a disposable private config; no token is passed in argv. Signing and exact
identity/issuer verification are separate steps. Retrying the same inputs is
idempotent. The bootstrap image must be published and qualified before use.
