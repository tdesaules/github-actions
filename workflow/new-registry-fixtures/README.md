# New registry fixtures

Fetches the public CA from the fixed private `test-registry` service, verifies TLS
readiness, creates ephemeral Cosign keys and optionally seeds/signs the synthetic
OCI fixture. Private key ownership is assigned to the runner UID. No engine
socket or host registry process is used.

Inputs: `directory` (default `.build/fixtures`), `seed`, `signed-fixture`.
Outputs `repository`, `reference`, `destination`. CLI flags override `INPUT_*`.
Only the CA retrieval uses private HTTP; image operations use verified TLS.

The fixed Go/Skopeo/Cosign scratch image was published in `1.2.0`.
