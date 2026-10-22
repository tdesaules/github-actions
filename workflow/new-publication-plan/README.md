# New publication plan

Validates downloaded qualification artifacts before publishing. Recomputes the
expected image matrix, requires both architectures at the exact checkout revision,
rejects duplicate/missing/untested metadata and verifies archive and OCI blob
checksums. The resulting index contains the exact tested platform manifests.
Unchanged images retain their pinned references.

Inputs: `directory`, `owner`, `release-plan`. An empty release plan requires a
complete bootstrap set. Writes `publication.json` and exact index manifests.
This fixed Go action publishes nothing. Its scratch image contains Git and
Skopeo and reuses the published matrix executable for source selection.

Published in `1.1.0`, the action is called directly by the release job.
