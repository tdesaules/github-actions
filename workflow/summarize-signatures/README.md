# Summarize advisory signatures

Record actual Cosign step outcomes against an immutable OCI image or artifact.
`success` alone does not imply signing: the producer must also emit `signed=true`.
Verification requires a successful verification step and a Cosign JSON claim
matching both the subject repository and manifest digest.

Missing credentials, disabled/skipped signing, failed tools and missing or
mismatched verification reports are retained as `not_run` or `error`, with
warnings. They do not create a security-only release gate. Persist this summary
and the public verification report in a separate advisory artifact upload.

The static entrypoint runs in scratch and never receives a signing key.
The source Containerfile reference is bootstrap-only until native qualification
and immutable image publication are complete.
