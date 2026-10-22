# Test catalog vulnerabilities

Refreshes Grype's database once and scans both architectures of every discovered
catalogue image. Missing SBOMs fail the run. Retains database metadata, full Grype
reports and advisory-review evidence. Continues remaining scans after a failure.

Inputs: `directory` (default `.build/maintenance`), `policy` (default
`grype-review.json`). The severity gate is `high`. Go advisory exclusions require
nonempty symbol evidence for every copy and absence of the affected package.
Patched npm exclusions require unique exact SHA-256 evidence for every walker in
every copy; missing, altered, ambiguous or wrong-version evidence fails closed.

The fixed Go/Grype scratch image was published in `1.1.0` and is called directly
by the maintenance job.
