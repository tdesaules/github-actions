# Test Pantavisor platform

Runs the System VM harness with checksum-pinned QEMU 10.2.2. Includes the exact
runtime tools for firmware, GRUB, ext4, SSH and rootfs preparation. No host engine
socket, board SSH credentials, host mount operation or KVM device is required.

The selected bundle is qualified using ephemeral private keys/disks. The action
copies only immediate JSON/log results into the separate uploader-readable
reports directory, including after test failure. Private fixture directories
must never be uploaded. System's base and k0s jobs run independently.

This tests emulated integration, not physical-board qualification. The hosted
runtime and both architectures still require execution before image release.
