# Test Atomic installer media

Bind an ISO build receipt to the expected source, candidate version and qualified
AMD64 OS archive. Check the ISO hash, byte size, implanted media checksum, bootable
BIOS/UEFI catalog entries and SHA-256 sidecar. Extract the embedded OCI layout,
compare its native manifest digest with the qualified OS and verify every
referenced blob through a digest-preserving Skopeo copy.

The original ISO hash is checked again before emitting a qualification receipt
and the exact ORAS payload list. The Fedora runtime needs no privileged mode or
mounted container engine. Allow temporary space for two embedded OCI copies.

Keep the original build receipt next to the ISO as `ISO.build.json`. It is
included in the ORAS payload together with the ISO and checksum sidecar, so tag
verification can consume the original source binding rather than inventing a
new build receipt. Qualification reports identify the current test run and retain
the original build run separately. On tag requalification, use the original OS
archive hash from release evidence; newly serialized registry-download tar files
may have different transport hashes while retaining the same native manifest.

`tested: true` means installer-media checks passed. `boot_tested: false` explicitly
excludes VM installation, Secure Boot, graphical login and physical hardware.
Those remain separate qualification evidence.

The source Containerfile reference is bootstrap-only until native qualification
and immutable image publication are complete.
