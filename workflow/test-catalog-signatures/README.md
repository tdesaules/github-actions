# Test catalog signatures

Verifies all platform and index signatures, including reused images, against the
repository's main workflow identity and GitHub OIDC issuer. Writes per-digest
verification evidence before marking each journal entry signed.

With `tag`, also requires the existing SemVer tag to match the checkout and be
reachable from `origin/main`, discovers pinned images from source metadata and
verifies their index digests and both architectures. This validates release tags
without rebuilding or changing them.

Inputs: `directory`, `repository`, optional `tag` and registry credentials.
The fixed Go/Git/Skopeo/Cosign scratch image is being qualified before activation.
