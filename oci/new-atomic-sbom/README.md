# Source-bound Atomic rootfs SBOM

**BOOTSTRAP ONLY:** `image: Containerfile` must become the exact native-qualified
published index before consumer activation. Local builds are not hosted
qualification or publication.

A fixed-operation Go Docker action for Fedora/RPM Atomic images. It verifies an
OCI source, unpacks its complete visible filesystem with rootless umoci, and runs
the pinned Syft scanner with the full image cataloger set. It does not execute
target-OS software, use an engine socket, mount devices, or launch privileged
workers. Static cross-architecture scans are supported.

This avoids Syft's cumulative image-layer indexing for large OSTree images.
The original registry-scanning action remains available for other workloads.

## API

| Input | Meaning |
| --- | --- |
| `archive` | Native OCI archive, relative to workspace; exclusive with `reference` |
| `sha256` | Required exact transport checksum when using `archive` |
| `reference` | Immutable `registry/repository@sha256:...`, native manifest or index |
| `architecture` | Required `amd64` or `arm64`; checked against image config |
| `revision` | Optional expected 40-character OCI revision label |
| `output` | New workspace-relative SPDX file; default `.build/security/atomic.spdx.json` |
| `binding` | New workspace-relative report; default `<output>.binding.json` |
| `registry-username` | Optional registry user, environment only |
| `registry-password` | Optional job token/password, environment only |

Outputs: `sbom`, `source-binding`, `sbom-sha256`, `manifest-digest`,
`architecture`, `rpm-packages`, `rpm-covered`. Paths are workspace-relative.
Output format is fixed to SPDX JSON; there is no arbitrary operation selector,
scope/exclusion input, shell command, or target execution mode.

The two credential inputs map to `INPUT_REGISTRY_USERNAME` and
`INPUT_REGISTRY_PASSWORD`, never CLI flags. Skopeo receives only a mode-0600
temporary auth-file path; that file is removed immediately after copying.

Archive inputs must be regular files. Traversal, escaping symlinks, unsafe output
ancestors and aliasing source/output paths fail before scanning. Both output
paths must be fresh. Existing user evidence is not overwritten or deleted;
failures emit no new success outputs. Use attempt-specific paths and gate
downstream consumers on the actual step outcome rather than stale file presence.

## Integrity and complete coverage

1. Validate the archive checksum, or hash the registry response against the
   requested immutable digest. An index must select one unambiguous requested
   Linux architecture; the native child must be an OCI image manifest.
2. Copy using Skopeo `--preserve-digests`. Verify the native manifest, config and
   every layer blob's hash/size before unpacking. Native gzip, zstd and
   uncompressed OCI layers are accepted; none are forcibly recompressed.
3. Use `umoci unpack --rootless` into container-private disk scratch. There is no
   fixed 20 GiB tmpfs allocation or workspace-rootfs export.
4. Query a copy of the extracted RPM database using the scanner runtime's RPM
   executable. Target binaries, scripts and scriptlets are never invoked.
5. Scan the entire rootfs with explicit `default-catalogers: [image]`, empty
   `select-catalogers`/`exclude`, default image scope, serial cataloging and
   `GOMEMLIMIT=4GiB`. Ambient Syft configuration and credentials are not inherited.
   OSTree objects, non-RPM ecosystems and additional RPM-typed ELF records remain
   included. This restores the original image cataloger set for directory input;
   it is not an RPM-only scan.
6. Require valid SPDX document metadata and every installed RPM
   name/epoch/version/release/architecture identity in the unfiltered SBOM.
   Architecture-less GPG key records are normalized, not discarded. Additional
   filesystem packages are retained rather than filtered to the installed DB.
7. Rehash original archives after scanning, bind the exact SBOM checksum, and
   publish the two reports exclusively. Failed transfers remove only their own
   partial outputs. Private layers/rootfs/authentication files are cleaned up.

The compressed temporary layout is removed after extraction. Only the SBOM and
binding are exposed as outputs; input archives remain unchanged. New output
directories/files are owned by the runner UID for later artifact transfer.

## Binding schema

Schema 1 records `operation: atomic-rootfs-sbom`, the original archive/checksum or
immutable reference/requested digest, native manifest descriptor, architecture,
Linux OS, optional valid source revision, config descriptor, all layer descriptors,
SPDX path/checksum/size/package count, static RPM inventory hash and coverage
counts, fixed cataloging settings and the scanner's measured peak process RSS.

It does not dump image environment, arbitrary labels, registry credentials or
authentication configuration. The SBOM describes a verified directory view with
the native digest as source version; it does not fabricate original image layer
annotations. Preserve the binding beside the SBOM to retain original OCI identity.
This report is neither OS functional qualification nor a claim of vulnerability
absence, and does not replace existing release qualification receipts.

## Resource behavior and advisory integration

All tools have a common 45-minute cancellation deadline. SIGINT/SIGTERM kill tool
process groups, with bounded pipe shutdown and cleanup of private scratch.
Source downloading/extraction is disk-backed inside the ordinary container.
Retain enough runner disk for the input archive, one compressed layout during
unpack, and the complete extracted rootfs.

Syft is fixed to `GOMEMLIMIT=4GiB` and `SYFT_PARALLELISM=1`. Its exact Linux
`wait4` peak RSS is recorded and must be positive and at most **7 GiB** before
outputs are accepted. This is a process-RSS qualification budget, **not a claim
that native GitHub `uses:` can set Docker's cgroup memory options**. Local
qualification additionally runs the entire action with a hard 7 GiB/no-swap
container limit. Filesystem page cache and whole-runner usage are distinct.

Action errors are real nonzero exits. Consumer SBOM/Grype steps should use
`continue-on-error: true`, retain actual outcomes, and run Grype only after a
complete SBOM was produced. Findings and scanner failures stay advisory.
The catalog's positive scanner qualification is blocking because it tests the
product's functionality, not whether its subject has vulnerabilities.

## Tool provenance and qualification

- Final/build base: explicit-major `quay.io/fedora/fedora:45`, per repository policy.
- Go build RPM: `golang-1.27.4-2.fc45`, available for x86_64 and aarch64.
- Skopeo, umoci, RPM and trust roots retain Fedora's runtime dependency closure.
  Availability was checked for both native architectures.
- Static Syft **1.54.1** and its Apache-2.0 license are copied from catalog 1.7.3's
  native-qualified scratch runtime:
  `ghcr.io/tdesaules/github-actions/oci-new-sbom@sha256:803dcae4aec52c76bfb3da34106339a660b98f2e018fe35e8d3dffe9c2a4e353`.
  The donor's build source is `shared/runtime/compile-syft.sh` at
  `45c9ce9160b50d49a7167543f2286e000656e565`.

The prior full-rootfs proof, preserved under
`/tmp/opencode/sbom-memory-review/ROOTFS_REPORT.md`, matched all 7,999 filesystem
package identities and 84,252 file/checksum records of the exact published AMD64
image, including 4,175 OSTree-path files and the retained nano ELF. Syft used
1.68 GiB observed cgroup peak; subsequent Grype retained 369 vulnerability matches.
That proof motivates this action but does not qualify its newly built image.

Run focused tests and vet from the catalog root:

```text
go test ./oci/new-atomic-sbom
go vet ./oci/new-atomic-sbom
python3 -m unittest discover -s tests/unit -p test_atomic_sbom.py
```

`ATOMIC_SBOM_TOOL_INTEGRATION=1` enables a real Skopeo/umoci zstd extraction test
inside the built action runtime. Missing tools fail this explicit test; it is
separate from the mandatory large real-OS hosted fixture, not a replacement.

The generic native runtime harness already invokes `--tool-version` for new
actions; no dispatcher change is needed. Dedicated `image.yml` rows use the
published compressed fixture exporter and candidate `uses:` on both native
architectures, reject source tampering/architecture mismatch, and inspect the
retained SBOM/source binding against the original fixture checksum, source SHA,
at least 200 real layers and at least 1,000 covered RPM records. Only source
revision metadata is relabelled by that qualification fixture, not OS file content.
