# Action-composed orchestration

The orchestration migration is in progress. The `lint` job now assembles the
execution planner and source-check composite with `uses`, without host commands.
The other jobs still use Python orchestration and direct container invocations;
the new planner is not yet their authoritative selector.

All container action references, including the lint prerequisites, are pinned
to the digests published by run `39144169525` and recorded in tag `0.3.0`.
Hosted qualification run `39138914416` succeeded for all 48 native variants,
including the corrected Markdown and Renovate images. Markdown includes only the
markdownlint library dependency
closure; Renovate uses a source-verified depth guard for the unpatched upstream
braces advisory.

## Implemented building blocks

- `workflow/get-execution-plan`: discovers images from action metadata and build
  dependencies from Containerfile COPY/ADD instructions; emits native AMD64 and
  ARM64 matrices with separate test selection and rebuild flags.
- `workflow/new-candidate-action`: writes temporary metadata and a Dockerfile
  referencing an already loaded local image, or a direct native Docker action
  reference for an immutable registry digest. It does not load images itself.
- `workflow/import-candidate-image`: transfers the archive with Skopeo into the
  isolated `candidate-registry` workflow service, preserving its digest. This
  provides a socket-free path for native tests; its transfer operation passed on
  both hosted architectures in run `39138914416`.
- `workflow/new-test-fixtures`: produces deterministic OCI archives and positive
  and negative source fixtures.
- `workflow/new-image-metadata`: checks archive digests, layer checksums, platform
  and `/tmp` permissions, then records qualification metadata. Invoke it only
  after tests and vulnerability checks succeed.
- `workflow/test-source-files`: assembles the check actions from a JSON
  check selection. This repository-internal composite requires this catalogue
  checked out at the workspace root because its `uses` paths are local.
- `check/test-markdown` and `check/test-links`: explicit literal file lists,
  markdownlint and Lychee, respectively.
- `check/test-action-metadata`: validates native action ownership, input
  forwarding and the action-only composition contract.
- `maintenance/update-dependencies`: explicit-repository Renovate execution in a
  scratch image, with Git, Node and Go runtime dependencies included.

Every container action has its own executable, Containerfile and metadata.
The Buildah host launcher now forwards literal arguments with a narrowly scoped
Docker invocation rather than a host Python script.

## Selection contract

Documentation edits select Markdown and link checks. A Containerfile edit
selects its image on both native architectures. Action metadata changes select
tests without rebuilding. Shared COPY dependencies select their consuming
images. Renames are supplied as deletion plus addition, and deleted files are
recorded but never passed to file checkers. Markdown and link configuration
changes select their dependent documents. Empty image matrices remain valid.

## Hosted lint activation

Run `39148591681` at commit `1064f08` passed with the action-composed `lint` job.
The planner selected action metadata, workflow syntax, YAML, Markdown and links
for that change. The composite executed those five checks and skipped
Containerfile, shell and TOML checks because their selections were absent.
All selected checks passed using the published digest-pinned images.

The `lint` job contains only checkout, `get-execution-plan` and
`test-source-files` action invocations. Pushes compare the event's base and target
commits; dispatch and tag runs request a full source check.

## Remaining integration

Artifact aggregation, release planning/finalization and registry signature
fixtures still need reusable interfaces. The other workflow jobs must
then consume the planner and composites, and qualification must demonstrate
selective hosted execution and unchanged digest reuse. Local tests do not
establish those hosted properties.

Renovate is not yet activated in maintenance. Provide a dedicated bot/App token
with repository contents, pull request and workflow update permissions; keep
hosted Renovate App execution disabled for this repository to avoid duplicate
updates. The release App credentials are not implicitly reused.
