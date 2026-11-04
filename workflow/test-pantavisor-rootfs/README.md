# Test Pantavisor rootfs

Runs the checkout's `build/smoke.py` against a verified component using SquashFS
and checksum-pinned QEMU 10.2.2 user emulation. The action contains its own native
QEMU binary on both action architectures; the component architecture is ARM64.

It verifies executable/version smoke tests, not arbitrary OCI service startup.
Functional host/LXC integration belongs to System qualification. Temporary
extraction is isolated; source files need only be readable.
