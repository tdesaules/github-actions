# Get image matrix

Discovers independent image owners from action metadata and Containerfiles.
Consumes changed paths, including deletions, and returns an AMD64/ARM64 matrix.
Metadata-only changes reuse published images; unpublished images are rebuilt.
An empty selection produces `{"include":[]}`. The workflow controls concurrency.

Inputs: `files` (JSON array), `full`, `event`, `ref`, optional `release-plan`,
and `reuse-artifacts`. The latter is only used after independent exact-source
artifact validation. A release plan must match the checkout revision.
Explicit flags override `INPUT_*` values. No build is executed by this action.

Its independent scratch image contains Git and a fixed Go executable. Published
with `1.0.1`, it is called directly by `check` after optional release planning and
artifact-source validation.
