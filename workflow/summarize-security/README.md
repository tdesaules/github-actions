# Summarize advisory security evidence

Fixed-operation Docker action producing a JSON report and Markdown job summary.
Use `if: always()` and `continue-on-error: true` when composing advisory steps.
Pass producer **outcomes**, not their coerced conclusions.

Required inputs: `subject` (64 lowercase SHA-256 hex digits, optionally prefixed
`sha256:`, or `git:` plus a full 40-digit commit SHA), `sbom-outcome`,
`scan-outcome`, `sbom`, and `report`. Outcomes are `success`, `failure`,
`cancelled`, or `skipped`. `output` defaults to `.build/security/summary.json`.
Paths are literal; output must be workspace-relative JSON, must not traverse
symlinks, and must differ from both report inputs.

JSON schema version 1 contains `subject`, `advisory: true`, and separate `sbom`
and `scan` records with `status`, `outcome`, `path`, and optional `reason`.
Grype records include `severity_counts`, optional `database` from `descriptor.db`,
and `database_age_seconds` when its `built` timestamp is parseable. Reports are
bounded to 32 MiB each. Supported SBOM shapes are SPDX, CycloneDX, and Syft JSON.

Statuses are `clean`, `findings`, `error`, or `not_run`. SBOM `clean` means a valid
SBOM was produced, not that dependencies are vulnerability-free. Failed/cancelled
steps remain errors even if a stale valid report exists; skipped steps are
`not_run`. Missing or malformed reports remain errors. Findings and producer
errors never fail this action; invalid inputs and unsafe/unwritable output do.
Markdown highlights actual High/Critical counts and tool failures. There is no
severity threshold and this evidence never asserts functional qualification.

Source metadata is temporary. Publish native tested archives and pin the observed
digest before consumer activation; follow the catalog source-bootstrap procedure.
