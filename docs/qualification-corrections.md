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
