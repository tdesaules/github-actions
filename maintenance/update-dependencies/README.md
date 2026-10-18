# Update dependencies

Candidate action running Renovate 44.167.1 for one explicit GitHub repository.
Required inputs: `repository` (`owner/name`) and `token`; `dry-run` defaults to
`false`. Authentication is passed through `RENOVATE_TOKEN`, never an argument.
The token needs contents, pull request and workflow update permissions.

The scratch runtime includes Node, Git and Go for this repository's dependency
managers. Autodiscovery is disabled; an existing Renovate configuration is
required. It has no host package manager or Docker socket dependency.

The upstream `braces` 3.0.3 AST walkers receive a depth guard for
GHSA-vfj7-8cjw-p6xm. The build verifies the original source hashes and tests the
advisory payload against all three patched walkers. Its original package version
is retained; vulnerability review requires all three patched source hashes in
the SBOM. Missing or modified evidence keeps the vulnerability blocking.

`--repository` and `--dry-run` override `INPUT_REPOSITORY` and `INPUT_DRY-RUN`.
The image is unpublished and maintenance integration is pending. Disable hosted
Renovate App execution before activating this runner to avoid duplicate updates.
