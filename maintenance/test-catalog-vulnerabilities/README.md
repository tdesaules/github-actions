# Test catalog vulnerabilities

Refreshes Grype's database once and scans both architectures of every discovered
catalogue image. Missing SBOMs produce real tool errors, continued by the workflow
as advisory security results. Retains database metadata, full Grype
reports and advisory-review evidence. Continues remaining scans after a failure.

Inputs: `directory` (default `.build/maintenance`), `policy` (default
`grype-review.json`). There is no severity exit gate. Go advisory exclusions require
nonempty symbol evidence for every copy and absence of the affected package.
Patched npm exclusions require unique exact SHA-256 evidence for every walker in
every copy; missing, altered, ambiguous or wrong-version evidence fails closed.

Each attempted component/platform scan writes a digest-bound status sidecar.
The Go/Grype scratch runtime uses an allowlisted Fedora builder. A source
bootstrap activates this contract until its new published digest is available.
