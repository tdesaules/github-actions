# New image metadata

The image is published and pinned by digest in tag `0.3.0`.

Invoke after successful tests and scans. Required inputs: `archive`, `image`,
`architecture` (`amd64` or `arm64`), `revision` (full Git SHA), `version` and
`output` (JSON file). Corresponding named CLI flags override `INPUT_*` values.

Checks manifest/config/layer SHA-256 digests, native platform and final `/tmp`
permissions. Writes archive checksum, image digest and qualification identity.
The `tested` field records the caller's successful qualification sequence; it
is not an independent attestation that tests were run.
