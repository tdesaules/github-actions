# Test reviewed vulnerabilities

Refreshes Grype's database explicitly, records its exact snapshot metadata and
scans the candidate SBOM using the independently generated review configuration.
Findings do not set a severity exit gate. Tool/database/configuration errors keep
their real failing exit status; the workflow makes this dedicated step advisory.
The raw report and `<output>.status.json` retain counts, subject digest and errors.
Temporary database caches are not included in qualification artifacts.

Inputs: `sbom`, `config`, `output`, `database-metadata`. Paths must be distinct;
an empty `config` explicitly requests an unreviewed scan with no exclusions.
An explicitly supplied missing/invalid configuration remains a real error.
CLI flags override `INPUT_*`. The fixed Go/Grype scratch runtime is built from an
allowlisted Fedora builder. Source bootstrap activates the reporting contract
until a new published digest replaces the older threshold-gated action image.
