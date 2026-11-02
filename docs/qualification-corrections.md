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
immutable artifact IDs. This exercised pagination and real GitHub metadata before
the hosted activation checks below.

## Active workflow contract

1. Pass `archive-output` to runtime qualification only for `scan && !rebuild`,
   retaining `.build/<image>/<arch>/scan.oci.tar`. Run SBOM, review and Grype steps
   for `rebuild || scan`, choosing `image.oci.tar` for builds and `scan.oci.tar`
   otherwise. Retain scan JSON diagnostics without producing publication metadata
   or qualified OCI artifacts for scan-only jobs.
2. Pass the release plan to artifact-source validation. Download its immutable
   `artifact-ids` instead of the legacy attempt pattern. Name new qualified
   artifacts `oci-<image>-<arch>-<run>` with overwrite enabled. Keep final archive,
   revision, version and complete-set validation in publication.
3. Validate the tag-triggered GitHub Release and maintenance against published pins.

## Hosted qualification evidence

Activation commit `6f0bd9c` passed run `41089752823`: 94 native action tests,
zero builds and zero qualification scans. That run established metadata-only
execution, not the scan-only or artifact-reuse paths.

Run `41104746535` at `ac8070b` exercised the policy clarification and checkout-bound
validator: 94 tests and scans passed, with only the two artifact-source variants
rebuilt. The other 92 variants were scanned from published images. Only the two
rebuilt variants produced qualified OCI artifacts.

The targeted AMD64 retry succeeded in attempt 2. Stable names preserved the ARM64
artifact from attempt 1 while replacing the AMD64 artifact:

- AMD64: `13003577305` replaced by `13094240509`.
- ARM64: `13002049514` retained unchanged.

Preparation run `41332828685` downloaded exactly IDs `13094240509,13002049514`,
verified and published those archives without builds, verified new and reused
signatures, and atomically pushed commit `795d8f4` and tag `1.3.1`. Tag run
`41333295085` verified the release signatures and published GitHub Release `1.3.1`.
The corresponding main run `41333295023` also succeeded.

The rebuilt validator was exercised locally: a mismatched checkout is rejected
before API access; the matching checkout and actual historical release plan select
94 distinct IDs from successful run `41038843468`. Unit cases also reject queued
or in-progress source runs, incomplete/expired sets and ambiguous artifacts.

## Maintenance and Renovate

Renovate authentication/extraction succeeded in run `41035011330` (190 detected
dependencies). Real execution in `41036488966` created PRs #1/#2 and dashboard #3.
Hosted-App disablement was confirmed by the repository owner. Those maintenance
runs failed at scanning older images, not at the Renovate step.

After publication of the fixes, maintenance run `41053499055` succeeded: SBOMs for
both architectures, refreshed Grype scans, link checks and authenticated Renovate
dry-run. The repository owner subsequently merged the Node and Go PRs. The Node
merge cancelled the preceding Go-only push run, so a separate full qualification
was required rather than reuse of the Node-only result.

## Full qualification with Go 1.27.3 and audit corrections

Preparation `41352590229` passed 94 builds and tests but refused its final push
because main advanced. This exercised the stale-source protection; it did not
produce a new release tag.

Run `41396008309` at source `3c892b9` subsequently passed all 94 native builds
and tests, published the exact archives, signed and verified their digests, and
atomically pushed commit `7a02e29` and tag `1.4.0`. This includes the private-token
signer and the Renovate source-checksum helper. Those images are now pinned.

Preparation used the signers pinned in its source checkout, so successful release
signing does not by itself establish execution of the newly published private-token
signing implementation. Keep that distinction when reporting OIDC validation.

Tag run `41407237512` verified signatures and published GitHub Release `1.4.0`;
main run `41407237711` succeeded. Maintenance `41407460725` passed catalogue SBOMs,
refreshed Grype scans, links and the new runner's authenticated Renovate dry-run
(167 dependencies detected). A real source-upgrade PR is still needed to establish
the bundled checksum hook's hosted write behavior.

Preparation `41420324041` reused the two Renovate archives from `41419110496`,
signed them using the private-token signer published in `1.4.0`, and pushed tag
`1.4.1`. Tag run `41421033026` and main run `41421032426` succeeded. This provides
real OIDC signing evidence for the token-file implementation.

Renovate's source-checksum PR path is still pending: maintenance `41421256070`
completed successfully at the Actions level but Renovate aborted with
`repository-changed`. See the [review findings](repository-review.md) for the
reproduced sanitized-environment problem and its local correction.
