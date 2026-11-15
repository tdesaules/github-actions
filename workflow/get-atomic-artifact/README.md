# Get current Atomic artifact

Select one native candidate or qualified artifact from the **current workflow
run**, including a successful producer from an earlier attempt when only failed
jobs are rerun. This action selects metadata; the official download action
downloads the selected immutable artifact ID. Downstream receipt, source,
platform, archive checksum and qualification checks remain mandatory.

## API

Required inputs: `repository` (`github.repository`), `run-id` (`github.run_id`),
`workflow-sha` (`github.sha`), `max-attempt` (`github.run_attempt`), `kind`
(`candidate` or `qualified`), `architecture` (`amd64` or `arm64`), and `token`
(repository-scoped `github.token` with `actions: read`).

Each explicit workflow identity input must equal its actual `GITHUB_*` environment
value. On tag workflows, pass the tag metadata commit SHA, **not** the OS build's
parent source SHA. The run API must confirm run ID, repository and workflow SHA.
Artifact-level workflow identity is checked when supplied by GitHub.

Outputs: `artifact-name`, `artifact-id`, `producer-attempt`.

Only exact `{candidate|qualified}-os-{amd64|arm64}-{run_id}-{attempt}` names are
eligible. The highest attempt at most the current attempt wins. Future attempts,
other runs, other architectures and other kinds are ignored. Duplicate eligible
attempts, missing artifacts, invalid identities and an expired newest match fail
closed. An expired newest artifact never causes fallback to an older artifact.
No cross-run development artifact reuse is possible.

The action paginates the fixed GitHub.com REST run-artifacts endpoint (100/page,
maximum 100,000 artifacts), rejecting incomplete or changing counts. It ignores
external pagination URLs, forbids redirects and API-origin overrides, and bounds
requests to 20 seconds each and the operation to 90 seconds. Tokens are forwarded
only in the environment and Authorization header; errors omit response bodies
and transport diagnostics. Deleted artifacts absent from the API cannot be
inferred; downstream receipts still enforce the selected artifact's provenance.

## Runtime and validation

`image: Containerfile` is **bootstrap-only**. Publish exact qualified native AMD64
and ARM64 images and pin the action image digest before consumer activation.
The static Go executable uses scratch, CA certificates from the existing pinned
Go builder and a mode-01777 `/tmp`; no shell or engine socket is required.
Build context is the catalog repository root.

```sh
go test ./workflow/get-atomic-artifact
podman build -f workflow/get-atomic-artifact/Containerfile -t localhost/get-atomic-artifact:bootstrap .
podman run --rm localhost/get-atomic-artifact:bootstrap --tool-version
```

Tests use an injected HTTP transport without exposing an alternate API URL in
the action. They cover rerun selection, pagination, duplicate/expired/missing
artifacts, foreign/future names, run/SHA/context validation and safe token errors.
Local tests and rootless container smoke tests do not prove hosted execution,
registry publication, credentials or native ARM64 qualification.
