# New catalog signatures

Signs each newly published platform digest and index with Cosign and GitHub OIDC.
Requires verified publication in the journal. Reused image signatures are retained
and independently verified by `test-catalog-signatures`.

Inputs: `directory`, `registry-username`, `registry-password`. Requires job
permission `id-token: write`. Registry credentials are passed only through the
environment. Transparency-log verification remains enabled.

The fixed Go/Cosign scratch image was published in `1.1.0` and is called directly
by the release job.
