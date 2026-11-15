# Verify Atomic release bundle

**BOOTSTRAP ONLY:** `image: Containerfile` is a source-build bridge. Activate
consumers only after both native action images pass hosted qualification and
the published index is pinned by its real digest.

Fixed-operation, static Go Docker action. The scratch runtime needs no engine,
network, mounted sockets or privileges. It verifies complete release qualification
before either OS index or installer publication.

## Contract

Inputs: `qualified-directory` (default `.build/qualified`), `source-sha`,
`version` (stable no-v SemVer), `output` (default `.build/atomic-qualified.json`).
The ISO publication list is derived from `files` in `iso-qualification.json`;
no inline cross-job step output is required.

Required layout:

```text
.build/qualified/
  amd64/
    qualification.json
    image.oci.tar              # actual basename comes from qualification.json
    iso-qualification.json
    installer.iso             # actual basename comes from iso-qualification.json
    installer.iso-CHECKSUM
    iso-build.json            # actual basename comes from receipt files
  arm64/
    qualification.json
    image.oci.tar
```

OS receipts require schema 1, component `os`, the expected architecture,
source/version, `tested: true`, `qualification: userspace`, `boot_tested: false`,
native manifest digest, archive checksum, checks and package inventory.
ISO receipts require `qualification: installer-media`, `boot_tested: false`,
size, original OS archive checksum and the same AMD64 native manifest.
Qualification `run_id` must equal `GITHUB_RUN_ID`; positive `run_attempt` may
be earlier than or equal to `GITHUB_RUN_ATTEMPT`. Cross-run and future-attempt
reuse fail. ISO `build_run_id`/`build_run_attempt` must match the original
untested build receipt.

The receipt's `files` array must contain exactly one ISO (`application/x-iso9660-image`),
its checksum sidecar (`text/plain`), and the original untested build receipt
(`application/json`), each with `path`, `sha256`, `media_type`. Paths are
flat basenames. Every transferred payload is rehashed; the ISO's size, sidecar,
build receipt and embedded-OS identity must agree.

Outputs:

| Output | Use |
| --- | --- |
| `bundle` | Verified receipt bundle JSON |
| `amd64_archive`, `arm64_archive` | Exact qualified OCI archive paths |
| `amd64_sha256`, `arm64_sha256` | OS transport checksums |
| `iso_directory`, `iso_files` | Pass directly to `oci/publish-artifact` |
| `iso_file`, `iso_sha256` | Installer filename and checksum |

Run this action **before** `oci/merge-image-index` and `oci/publish-artifact`.
Transfer receipts and payloads together; uploading only reports is insufficient.
Security reporting and signing remain separate advisory steps. This receipt
asserts functional/userspace/media qualification, never boot testing.

## Validation

`go test ./shared/atomicrelease ./check/test-atomic-release`

Shared fixtures cover wrong source/version/run/attempt, missing platforms,
tampered OS/ISO files, sidecars, original build receipts and manifest bindings.
