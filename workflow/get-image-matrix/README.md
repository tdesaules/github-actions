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
