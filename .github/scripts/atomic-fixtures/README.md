# Repository-only synthetic Atomic fixtures

**SYNTHETIC. NOT BOOTABLE. NOT OS QUALIFICATION. Never publish these as an OS,
installer or release.** This helper is repository test infrastructure, not a
discovered/published catalog action product. Its `tested: true` fields are
deliberately synthetic protocol metadata for testing consumers of receipts.
No Fedora/Niri/RPM/chroot userspace checks are executed or claimed.

The real installer BIOS/UEFI installation, welcome GUI and greetd/Niri evidence
was collected separately by the primary agent using the real approximately
4.97 GB ISO. Nothing generated here substitutes for that evidence.

## Inputs and outputs

- `directory`: NEW workspace-relative output directory; spaces supported,
  parent traversal, existing destinations and symlink ancestors rejected.
- `source-sha`: exact lowercase 40-hex fixture source revision.
- `version`: stable no-v version, default `1.0.0`.
- `large-sbom`: optional boolean, default `false`. Only the security-summary
  qualification row enables it; other rows do not generate the large document.
- Receipts use actual `GITHUB_RUN_ID` and `GITHUB_RUN_ATTEMPT`. Only when both
  are absent and `GITHUB_ACTIONS` is absent do explicit fixture-local defaults
  `1`/`1` apply. Partial/malformed hosted identities fail.

Generated output:

```text
SYNTHETIC-NOT-QUALIFICATION.txt
raw/
  SYNTHETIC-NOT-BOOTABLE.iso
  SYNTHETIC-NOT-BOOTABLE.iso-CHECKSUM
  SYNTHETIC-NOT-BOOTABLE.iso.build.json
  image.oci.tar
  os-manifest-digest.txt
qualified/
  amd64/
    image.oci.tar
    qualification.json
    SYNTHETIC-NOT-BOOTABLE.iso
    SYNTHETIC-NOT-BOOTABLE.iso-CHECKSUM
    SYNTHETIC-NOT-BOOTABLE.iso.build.json
    iso-qualification.json
  arm64/
    image.oci.tar
    qualification.json
transfer/
  payload.txt
zstd/
  fixture.oci.tar
  arm64.oci.tar
  fixture-checksums.json
```

Outputs `iso`, `build-receipt`, `os-archive-sha256`, `os-manifest-digest`,
`qualified-directory`, `iso-files`, `transfer-directory`, `transfer-files` and
`tampered-transfer-files` bind the exact files above. Descriptor outputs are
JSON arrays of `path`, `sha256`, `media_type`; the tampered array deliberately
substitutes an incorrect payload checksum without changing the actual file.

`zstd-directory` contains additional tiny AMD64/ARM64 archives with mixed zstd
and gzip layers, plus the checksum inventory consumed by the published
`workflow/test-index-result`. They exercise Skopeo's zstd descriptor annotations:
an assembled index must declare those annotations before its digest is chosen.
The fixture-only Zstandard writer emits one bounded raw block (RFC 8878);
it uses no external compression dependency and is not a production compressor.
These archives are separate from the synthetic OS/media receipt bundle.

With `large-sbom: true`, `security/large.spdx.json` contains approximately
34 MiB of streamed synthetic SPDX 2.3 file records. Each token is below 2 KiB;
the helper never constructs the complete array or document in memory. Document
metadata, unique record identifiers and the synthetic warning are deterministic.
Outputs `large-sbom` and `large-sbom-sha256` bind the original file. Both outputs
are omitted when the option is false.

The native summary row invokes the actual candidate CLI, then the published
Go-test action with `ATOMIC_SUMMARY_HOSTED_FIXTURE=1`. The dedicated
`workflow/summarize-security/hosted_fixture_test.go` verifies that the candidate
reported a clean/successful SBOM, a not-run/skipped scan, the expected subject
and path, and unchanged source bytes. Exit zero alone cannot qualify the
candidate: the previous binary emitted an advisory error for this size.
This fixture tests summary document handling, not real scanning or RPM coverage.

The base OCI archives contain real config/manifest/layer blobs for AMD64 and ARM64,
with source/version and synthetic labels. Their only rootfs file is `hello.txt`;
the mixed-compression archives additionally contain `gzip.txt`.
ISO `/niri-tomic-os` contains the exact AMD64 layout tagged `45`.
The ISO has valid implanted media checksum and BIOS/UEFI El Torito entries
marked bootable **at the catalog format level only**. Their dummy payloads are
not BIOS loaders, EFI executables or bootable systems.
Two MiB of labeled padding avoids an isomd5sum fragment-verification edge case
on sub-megabyte media. Generation verifies the implanted checksum before exposing
outputs; the resulting local test ISO was 2,494,464 bytes.

Each synthetic OS receipt has all eight current check names, explicit
`check NOT executed` values, a synthetic package entry, `tested: true`,
`qualification: userspace`, `synthetic: true` and `boot_tested: false`.
The synthetic installer qualification has the exact original three-file
payload list, original archive/manifest binding and separate current/build
run identities. Raw and qualified directories retain byte-identical original
untested `ISO.build.json`; it is never replaced with the synthetic qualification.

## Execution and integration

Build context is the catalog repository root. Fedora 45 runtime supplies xorriso
and isomd5sum; a static Go generator constructs deterministic tar/OCI metadata
without shared product-validation imports. Private staging keeps boot blobs
and temporary layouts out of the final declared fixture set. No shell command
interpolation, socket, mount operation or privileged mode is needed. Rootless
Podman and a non-root runtime UID are supported with a writable workspace.

The native pipeline should build this helper separately with qualified Buildah,
import the exact candidate image into its private registry and use
`workflow/new-candidate-action` to create
`.candidate-actions/.github/scripts/atomic-fixtures`. This directory is not a
catalog product; do not wire it into product publication or production releases.

Use `iso` and `build-receipt` as inputs to the real media checker, writing its
new result to a different path. Use `qualified-directory` for bundle parser
contract tests only. Use transfer descriptors for authorized test-registry ORAS
round trips and negative checksum cases. Never send this bundle to release
preparation/publication. There is deliberately no fake OS userspace executor.

```sh
go test ./.github/scripts/atomic-fixtures
podman build -f .github/scripts/atomic-fixtures/Containerfile \
  -t localhost/atomic-fixtures:test .
```

Unit tests inspect actual tar/config/blob relationships, exact file copies,
source/run bindings, deliberate tampering, spaces, path isolation and cleanup.
The real-media Go test runs when xorriso/isomd5sum are available; otherwise it
explicitly skips. Local rootless media tests do not prove hosted/native ARM64
execution or real installer bootability.

Observed local validation: all Go tests passed inside the rootless Fedora runtime,
including real ISO creation; host race tests and vet passed. Generation ran as a
non-root user with networking disabled and a directory containing spaces. Repeated
ISO/build-receipt bytes matched exactly. The current-source
`check/test-atomic-iso` image accepted the fixture and emitted all three payload
descriptors with `boot_tested: false`; the local `check/test-atomic-release`
image accepted the synthetic bundle. ORAS registry round trips and native hosted
AMD64/ARM64 execution remain pipeline work; no registry publication occurred.
