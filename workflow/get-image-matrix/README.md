# Get image matrix

Discovers independent image owners from action metadata and Containerfiles.
Consumes changed paths, including deletions, and returns an AMD64/ARM64 matrix.
Each row also exposes its source image reference for metadata-only tests. The
`test-registry-image` output is discovered from the registry service's action
metadata, without a separate image catalogue or lockfile.
Metadata-only changes reuse published images; unpublished images are rebuilt.
An empty selection produces `{"include":[]}`. The workflow controls concurrency.

`matrix` remains the complete qualification matrix. `build-matrix` contains only
rows with `rebuild: true`, plus one source reviewer dependency per architecture
requiring a scan. A reviewer already being rebuilt as a product is not duplicated.
Dependency-only rows carry `dependency: true`: their failures are advisory;
required product build failures remain blocking. `has-build` reports whether this
filtered matrix is nonempty. Published-image tests without scans allocate no
build jobs. Filtering does not alter qualification or publication selection.

Ignore-file changes are compared against the release's effective COPY/ADD/bind
inputs, using both Git trees and the same ignore precedence as the builder.
Removing an unused temporary-directory rule does not invalidate every image.
Copied ignore-file bytes, actual inclusion changes and unresolved dependencies
still require rebuilding.

On development pushes, `reuse-builds` can reuse build archives from the latest
20 completed push runs of this workflow on the same repository and branch.
Only successful per-image build jobs in the recorded attempt qualify, including
successful jobs in an otherwise failed run. The source commit must be an ancestor
with identical recipe and effective build inputs. Changed copied documentation,
deleted inputs and builder-launcher changes invalidate reuse too.

Artifact and job listings are paginated; expired, ambiguous, foreign or missing
artifacts are ignored. The source run is rechecked against concurrent retries.
API errors fall back to the original build plan. Reused rows retain `rebuild: true`
for candidate qualification but expose `artifact-id`, `source-run`,
`source-revision` and `source-version`, and are excluded from `build-matrix`.
The test job downloads the exact artifact ID, verifies its original build receipt
and OCI provenance, reruns qualification and retains the original source identity.
It never relabels an old archive with the current commit. Source reviewer helpers
are still built from the current source.

This cache is disabled for release preparation, artifact-source validation, tags
and full qualification. Release publication retains its exact-source checks.
`repository` and an Actions-read `token` are required when enabling reuse; the
token is passed only through the environment. Local Python selection remains
offline, mirrors effective ignore handling, and does not query GitHub for builds.

Inputs: `files` (JSON array), `full`, `event`, `ref`, optional `release-plan`,
and `reuse-artifacts`. The latter is only used after independent exact-source
artifact validation. A release plan must match the checkout revision.
Explicit flags override `INPUT_*` values. No build is executed by this action.

Its independent scratch image contains Git and a fixed Go executable. Until a
qualified digest exposing `build-matrix` and `has-build` is published, `check`
builds this control image on AMD64 and invokes it through the private candidate
registry. Tag validation still invokes the published digest without rebuilding.
After publication, remove the three selector bootstrap steps, restore the direct
`uses: ./workflow/get-image-matrix` call, and remove the temporary tag fallback.
