# Runtime and advisory-security transition

## Contract

- Prefer scratch for self-contained tools. Retain an allowlisted distribution
  for actions needing a complete interpreter/toolchain environment. All external
  stages and donors remain digest-pinned; the last stage is named `final`.
- Report legacy bases outside the allowlist explicitly. This transition migrates
  KAS and the newly adapted control-tool Containerfiles, not the whole catalogue.
- Security operations are advisory: SBOMs, reviews, vulnerability scans, signing
  and signature verification. Findings and operational failures remain visible.
  Grype's severity exit gate is omitted in catalogue scans. The low-level OCI
  action retains its historical `high` default for backward-compatible callers;
  catalogue/consumer calls explicitly select `fail-on: none`.
- Functional qualification remains blocking, including real negative tests,
  source/platform/checksum verification, tag ancestry, registry digest resolution,
  complete qualification sets, publication and atomic Git writes.
- `tested: true` means functional qualification, not security approval. A missing
  report, scanner outage or failed signature must never be described as clean,
  signed or verified.

## Reporting and publication

Dedicated security steps use step-level continuation; jobs and functional steps
do not. Summaries consume `steps.<id>.outcome`, not the coerced conclusion.
Scans retain digest-bound status/count sidecars and raw Grype/database evidence.
The step-outcome action emits the aggregate JSON and GitHub step summary, with
`findings`, `error` and `not_run` distinguished. Signature tooling records actual
per-digest outcomes; aggregate signing success alone does not assert that reused
images were signed again.

`workflow/test-catalog-publication` separates blocking tag/index validation from
advisory Cosign checks. Release finalization requires published artifacts but no
longer requires successful signatures. Mandatory archives/receipts are uploaded
separately from optional security diagnostics. Scanner/report upload errors do
not invalidate functional qualification.

The consumer template and System generator follow the same policy. Consumer
repositories are not activated by changing their generator: they still require
a genuinely published catalogue release with resolvable immutable references.

## Temporary source bootstrap

Existing released control images implement the old scratch/security contracts.
Changing their source does not change those immutable published images. The
workflow therefore uses `workflow/prepare-source-action` for:

- the catalogue validator accepting distribution runtimes;
- reporting-only candidate and maintenance scanners;
- advisory outcome summaries;
- independent tag publication validation;
- release finalization without a signature gate.

The bridge composes the existing Buildah, receipt, integrity, runtime-test,
registry-transfer and candidate-metadata actions. Every call is explicit and
uses a job-private registry digest. There is no host-script fallback or invented
GHCR digest. Auxiliary security-helper failure is advisory; a helper image
selected as a catalogue product must still build and pass functional tests.

These temporary control-tool builds also occur during tag verification; the
catalogue product images themselves are not rebuilt or republished on tags.
After publishing each new control action, pin its real digest and replace its
source invocation with the released action. Remove the bridge when no calls
remain. Hosted validation and actual publication are still distinct from local
builds/tests; no assistant push or remote release is implied by this transition.

## Local checks

Regression coverage includes unsigned-but-published release finalization, rejected
unpublished artifacts and racing refs, tag/source/index mismatches, real high
findings returning reporting success, scanner outage returning an error with
diagnostics, and continued/skipped steps never appearing as clean results.

The KAS migration's distribution and build evidence is documented separately in
[the builder README](../workflow/build-pantavisor/README.md). Historical Debian
advisory reports describe that former baseline, not the new Fedora image.
