# Qualification corrections awaiting publication

The updated sources add Containerfile-derived dependencies, an independent
`scan` matrix field, optional native archive retention in runtime qualification,
and immutable artifact selection across partial retries. OCI registry passwords
are forwarded through environment variables; Renovate covers npm and every
action family and supports a manual dry-run.

Publish the prerequisite images before activating their new workflow interfaces:

1. Qualify the changed images on both native architectures using the existing
   build/test workflow. Publish the exact qualified archives and verify signatures.
2. Update the immutable pins for `get-image-matrix`, `new-publication-plan`,
   `test-candidate-runtime` and `test-artifact-source` with verified index digests.
3. Pass `archive-output` to runtime qualification only for `scan && !rebuild`,
   retaining `.build/<image>/<arch>/scan.oci.tar`. Run SBOM, review and Grype steps
   for `rebuild || scan`, choosing `image.oci.tar` for builds and `scan.oci.tar`
   otherwise. Retain scan JSON diagnostics without producing publication metadata
   or qualified OCI artifacts for scan-only jobs.
4. Pass the release plan to artifact-source validation. Download its immutable
   `artifact-ids` instead of the legacy attempt pattern. Name new qualified
   artifacts `oci-<image>-<arch>-<run>` with overwrite enabled. Keep final archive,
   revision, version and complete-set validation in publication.
5. Run authenticated Renovate maintenance in dry-run mode, confirm hosted-App
   disablement, then execute real updates. Validate release preparation, signature
   verification, atomic commit/tag push and the tag-triggered GitHub Release.

These interfaces are implemented in source, but their activation and hosted
validation are not implied by local unit tests or a successful development run.
