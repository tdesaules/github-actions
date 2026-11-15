# Publish OCI artifact

Publish qualified files to GHCR using ORAS, preserving payload formats and
explicit media types. The input is a JSON array of `path`, `sha256`, and
`media_type` objects. Paths are flat names relative to `directory`.

Checksums must come from prior functional qualification. This action checks them
before publication, downloads the published immutable reference, checks the
downloaded bytes, and checks the original files again. It does not claim to test
an operating system or installer. Any registry or integrity failure is blocking.

Use a unique publication tag. The caller owns release serialization, tag policy,
and promotion of aliases. Each payload must be at most 10,000,000,000 bytes.
Larger ISOs need a separately qualified chunking/reassembly format.
Allow temporary disk space for a complete verification download.

Credentials travel through the environment and stdin to a private, automatically
removed ORAS registry configuration. No Docker socket or host tool is used.
The static Go entrypoint and ORAS run in scratch with CA certificates.

## Bootstrap status

The source Containerfile reference is temporary until native AMD64/ARM64 image
qualification and exact-archive publication provide its immutable runtime digest.
Do not activate consumers as released actions before that step.
