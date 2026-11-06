# Test step outcomes

Also summarizes steps whose IDs start with `security-`, using their real
`outcome` rather than a continued step's `conclusion`. Set `SECURITY_REPORT` to
the Grype report, `SECURITY_OUTPUT` to the summary JSON path and optionally
`SECURITY_SUBJECT`, `SECURITY_COMPONENT`, `SECURITY_ARCHITECTURE` for attribution.
`SECURITY_SUBJECTS` is an optional JSON map from step IDs to their individual
subjects, for example when image and component signatures have different digests.
The subject defaults to the sibling Syft SBOM's manifest digest when available.
For directory-based component scans, `SECURITY_RECEIPT` can identify the already
verified artifact inventory; its hash is explicitly labelled `receipt-sha256`,
not misrepresented as an OCI manifest digest.
Missing/corrupt scan evidence is an error, never clean. Summaries are appended
to `GITHUB_STEP_SUMMARY`; invoke this reporting step with step-level continuation.
The existing expected-failure assertions remain functional and blocking.
The `verified` output refers only to those outcome/file assertions, never to
security approval or signature verification.

Asserts that specified native action steps actually failed, rather than succeeded
or skipped, and that failed operations left no success outputs.

Inputs: `outcomes` (serialized GitHub steps context), `expected-failures` and
`absent-files` (JSON arrays, default `[]`). Outputs `verified`. Explicit flags
override `INPUT_*`. No shell or arbitrary command is executed.

The fixed Go scratch image was published in `1.2.0` and is called by `test`.
