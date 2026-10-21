# New GitHub release

Creates a GitHub Release through the HTTPS API only after its SemVer tag exists.
An existing release is retained without modification. Prerelease tags produce
prereleases. The workflow must first verify the catalogue tag and signatures.

Inputs: `repository`, `tag`, `notes` (default `CHANGELOG.md`), `token`.
The token is environment-only. Outputs `created`. Redirects are rejected.

The fixed Go scratch image is being qualified before activation.
