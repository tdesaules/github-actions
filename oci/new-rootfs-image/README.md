# New rootfs image

Wraps a verified Core/k0s `rootfs.tar.gz` in a single-layer ARM64 OCI archive.
Computes the uncompressed diff ID, preserves the original compressed layer and
uses the exported runtime contract's entrypoint and the system-container root
user. The local producer Containerfiles mirror this contract. It executes
no target code and therefore works on an AMD64 build runner without emulation
or a privileged engine. Qualification still runs on ARM64.

Returns the archive and root manifest digest. Output must not exist. Only this
operation's incomplete output is removed on failure. The image and component are
different distribution objects, with distinct digests.
