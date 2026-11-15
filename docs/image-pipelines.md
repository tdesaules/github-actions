# Independent image pipelines

The catalogue keeps a single Git version and three entry workflows:

- `main.yml`: branch pushes, with incremental source selection.
- `release.yml`: manual preparation on `main`, or stable SemVer tag verification.
- `maintenance.yml`: schedules and manual maintenance on `main`.

Both development and release call `catalog.yml`. Its graph is
`check → lint → images → test → release`. `images` is a bounded matrix with one
entry per discovered image, calling `image.yml`. No catalogue manifest is stored.

Each child runs `check → lint → build → test → publish`. Build and test use native
AMD64/ARM64 runners inside that child. Publication waits for both architectures of
that image; it does not wait for other images. Common repository checks and
documentation lint run in the parent; image-local lint runs in the child.
The parent test job runs repository contracts and verifies aggregate publication
evidence before catalogue finalization.

## Selection

Development compares the push's source changes, not all changes since the last
release. Changing `lint/actionlint/main.go` selects its image; a README-only change
does not start an image chain. Source-derived COPY/ADD/bind-mount dependencies
still select all affected images when shared inputs change. Renames and deletions
remain inputs to selection. An unavailable diff conservatively selects all work.

An action metadata change tests its existing digest without rebuilding, unless
the image is unpublished. Changes to qualification fixtures or the child workflow
select their affected test consumers. A scan-policy-only change runs the child's
advisory `scan` job instead of runtime qualification and creates no tested receipt.
The published reviewer is used without an extra reviewer build.

The parent retains `execution-plan-<run-id>` with the source SHA, matrices and
per-image selection reasons. A genuinely empty image selection skips the matrix.

Preparation compares against the previous release and qualifies the complete
intended catalogue, including unchanged digests. A tag qualifies the complete
published catalogue with an empty build matrix. No publication runs on development
pushes or tags.

## Publication and retry

The child `publish` job downloads only its image's qualified archives, verifies
their checksums/source/platforms, assembles the index and copies the exact content
to GHCR. Unchanged images produce verified reuse receipts without an image push.
Signing and verification are advisory.

Each child uploads `publication-<image>-<run-id>`. Its scoped `publication.json`
records the image, source, workflow run, catalogue version, platform digests and
publication result. Stable artifact names with overwrite support retries within
the same run; diagnostic artifacts additionally include the attempt number.

The parent rejects missing, duplicate, stale, wrong-run or unpublished receipts
and rechecks the remote index/platform digests. Only a complete global publication
can be finalized. A child receipt cannot authorize a catalogue Git write.

Preparation updates all action references in one writer and atomically pushes the
catalogue commit/tag. The subsequent tag execution creates the GitHub Release.
Individual images have no independent Git release or catalogue SemVer OCI tag.
If one chain fails, successfully published image bytes can remain in GHCR, but no
global release is finalized. Digest-preserving publication is idempotent on retry.

## Activation prerequisite

Source implementation, published action binaries and hosted execution are distinct.
The workflow changes require newly qualified and published versions of:

- `workflow/get-changed-files` (scoped/common checks and JSON check outputs);
- `workflow/get-image-matrix` (image-level matrix, scoped selection, tag tests,
  maintenance validation, scan-only flag and execution-plan output);
- `workflow/new-publication-plan` (scoped preparation and receipt aggregation);
- `workflow/publish-catalog-images` (scoped publication);
- `workflow/complete-release` (run-bound aggregate validation);
- `workflow/new-catalog-signatures` and `workflow/test-catalog-signatures`
  (scoped receipts and release-workflow signing identity).

Existing digest references must be replaced with verified digests of these
implementations **before activating this orchestration on GitHub**. Updating an
`action.yml` input does not upgrade its pinned executable. Do not point these
actions at mutable tags or bypass qualification with host Go/Python steps.

Roll out the control-action source changes and qualify/publish them using the
existing orchestration first, then activate the workflow split with the resulting
pins. The parent and child must use the same selector implementation. When a
publication planner itself is rebuilt, its qualified native candidate is used in
its child publication; other children require the already activated compatible
planner.

Before remote activation, verify the source-push workflow permissions, native
runner availability, GHCR access and repository-scoped bot App configuration.
Validate a branch selection, manual preparation, tag verification and manual
maintenance on hosted runners. Local tests and actionlint do not prove those paths.
