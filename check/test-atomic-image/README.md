# Test Atomic desktop image

Qualify the exact Niri Tonic OS archive on its native AMD64 or ARM64 runner.
Verify the input checksum, OCI architecture and source revision; unpack a
disposable root with Skopeo/umoci; execute Niri configuration validation, PAM
library checks and package/service/profile checks in that root; then verify the
original checksum again.

The output records the RPM inventory and userspace checks. `tested: true` means
these checks passed. `boot_tested: false` explicitly excludes boot, graphical
login, hardware and installer qualification. SBOMs and vulnerability scans are
separate advisory operations.

The Fedora runtime supplies Skopeo, umoci and chroot. It needs Docker's ordinary
chroot capability, not privileged mode or a mounted engine socket. Space is
required for the OCI layout and extracted root in addition to the source archive.

## Bootstrap status

The source Containerfile reference must be replaced by the exact native-qualified
multi-platform image digest before enabling released consumers.
