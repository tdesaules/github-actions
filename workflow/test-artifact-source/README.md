# Test artifact source

Validates a source run using the GitHub API before archive reuse. Requires the
exact requested revision, main branch, successful catalogue workflow and allowed
event. Returns an artifact pattern restricted to the verified run and attempt.

Inputs: `run-id`, `repository`, `revision`, `token`. CLI flags override `INPUT_*`
values except the secret token, which is forwarded only through the environment.
HTTP redirects are rejected; TLS verification remains enabled.

The independent scratch image runs a fixed Go executable. No host GitHub CLI or
Python is needed. Published with `1.0.1`, it is called directly by `check` when
an artifact source run is requested.
# Retry-aware artifact selection

The updated source accepts `release-plan` and returns immutable `artifact-ids`
for every required image/architecture. It paginates the artifact API, accepts
stable per-run names and legacy attempt names, selects the latest legacy attempt
per key, and rejects incomplete, expired or ambiguous sets. The source run must
remain completed and successful at the exact revision throughout selection.
Publication still verifies archive hashes, source revision and qualification.

Activate this interface only after publishing the updated image. The currently
pinned image exposes the legacy `artifact-pattern` output.
