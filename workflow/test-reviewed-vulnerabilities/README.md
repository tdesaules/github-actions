# Test reviewed vulnerabilities

Refreshes Grype's database explicitly, records its exact snapshot metadata and
scans the candidate SBOM using the independently generated review configuration.
The threshold remains `high` and is not configurable by this qualification action.
Retains the Grype report on threshold failure. Temporary database caches are not
included in qualification artifacts.

Inputs: `sbom`, `config`, `output`, `database-metadata`. Paths must be distinct;
CLI flags override `INPUT_*`. The fixed Go/Grype scratch image is awaiting
qualification and immutable publication.
