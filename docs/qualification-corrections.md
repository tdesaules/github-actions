# Qualification corrections and activation

The updated sources add Containerfile-derived dependencies, an independent
`scan` matrix field, optional native archive retention in runtime qualification,
and immutable artifact selection across partial retries. OCI registry passwords
are forwarded through environment variables; Renovate covers npm and every
action family and supports a manual dry-run.

## Published prerequisites

Run `41038843468` qualified the native variants at source `243c22c`. Preparation
run `41049790005` reused its 94 qualified archives, published and verified their
digests and signatures, then atomically pushed commit `f92fa51` and tag `1.3.0`.
The action metadata now pins the published images supporting the new interfaces.
The published artifact-source validator was also executed locally against the
successful retried run and its actual release plan: it selected 94 unique
immutable artifact IDs. This exercises pagination and real GitHub metadata, but
does not replace hosted verification of the new download wiring.

The workflow activation adds independent scans and immutable artifact downloads.
Its hosted validation still requires pushing the activation commit. Check these
cases before calling the migration complete:

1. Pass `archive-output` to runtime qualification only for `scan && !rebuild`,
   retaining `.build/<image>/<arch>/scan.oci.tar`. Run SBOM, review and Grype steps
   for `rebuild || scan`, choosing `image.oci.tar` for builds and `scan.oci.tar`
   otherwise. Retain scan JSON diagnostics without producing publication metadata
   or qualified OCI artifacts for scan-only jobs.
2. Pass the release plan to artifact-source validation. Download its immutable
   `artifact-ids` instead of the legacy attempt pattern. Name new qualified
   artifacts `oci-<image>-<arch>-<run>` with overwrite enabled. Keep final archive,
   revision, version and complete-set validation in publication.
3. Validate the tag-triggered GitHub Release and maintenance against the new pins.
   Renovate authentication/extraction succeeded in run `41035011330`; its real
   execution in `41036488966` created PRs #1/#2 and dashboard #3. Hosted-App
   disablement was confirmed by the repository owner. Those maintenance runs
   failed at scanning the older published images, not at the Renovate step.

Local unit tests and static workflow checks do not establish hosted execution of
the activated scan-only and cross-attempt reuse paths.

## Targeted qualification after activation

The next source change clarifies the existing fail-closed policy justification
without changing advisory IDs, hashes or severity, and binds artifact selection
to the actual checkout revision. Local selection of this change produces 94 native
test jobs: 92 scan-only variants and the two rebuilt artifact-source variants.

After pushing this change:

1. Confirm the 92 published variants run SBOM/review/Grype steps with no builds
   and no publication metadata. Only artifact-source should upload qualified OCI
   archives, one per architecture. Inspect retained JSON diagnostics.
2. Record those two artifact IDs, rerun only one successful artifact-source test
   job, and verify its stable artifact name receives a new ID while the other
   architecture retains its original ID. Require the complete run to succeed.
3. Prepare release on the exact same main revision using this source run ID.
   Verify the validator selects both IDs, including the retained earlier result,
   and publication uses the verified archives without rebuilding.
4. Verify signatures, atomic references and the resulting tag-triggered release.

The rebuilt validator was exercised locally: a mismatched checkout is rejected
before API access; the matching checkout and actual historical release plan select
94 distinct IDs from successful run `41038843468`. Unit cases also reject queued
or in-progress source runs, incomplete/expired sets and ambiguous artifacts.
