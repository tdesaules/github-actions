# Independent image pipelines

## Atomic integration qualification

Atomic OS actions are being integrated on the dedicated
`niri/atomic-release-integration` branch. Source `image: Containerfile` metadata
is bootstrap-only until native qualification and immutable publication complete.

- The image verifier uses the published `new-test-fixtures` compressed-layer
  exporter. This avoids VFS snapshots of the real 269-layer OS.
- The dedicated Atomic BuildKit constructor is tested separately against that
  real parent on native runners. Its runner-socket access is an explicit
  user-approved exception; the catalog's existing Buildah launcher is unchanged.
- Lorax has a separate explicit privilege exception. Its coordinator launches
  the same immutable local image as a worker with workspace and output mounts,
  without forwarding credentials or the engine socket to the worker.
- Small fixtures under `.github/scripts/atomic-fixtures/` exercise media and
  receipt protocols only. Their dummy BIOS/UEFI payloads are not bootable;
  synthetic receipts must never be published as OS qualification.
- Local real-media evidence includes an installer of 4,966,055,936 bytes,
  matching embedded OCI blobs, and BIOS/UEFI installation and greetd/Niri startup.
  This does not replace hosted native action qualification, production ORAS
  transfer checks, or physical hardware and graphical user-login testing.

Consumer activation still requires the published action references and observed
App-token, release, and retry execution. Development qualification does not
authorize production image publication.

Run [43752054926](https://github.com/tdesaules/github-actions/actions/runs/43752054926)
executed the renderer, real native OS userspace checks and real BuildKit fixture
construction successfully on AMD64 and ARM64. ORAS transfer positives also passed.
The run as a whole failed: nested JSON transport broke subsequent assertions,
cancellation fixtures assumed a `sleep` executable, the ARM64 Lorax runtime
requested an x86-only Syslinux dependency, and one newly uploaded artifact was
not immediately visible in REST. These failures require a new hosted run; none
of those partial successes alone constitutes complete publication qualification.

## Orchestration

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

A generated release commit does not start development image chains when its
stable release tag points to that exact single-parent commit and its diff contains
only `CHANGELOG.md` and image-reference promotions in existing action metadata.
Changing inputs, arguments, source files, ownership, or including another commit
in the push retains ordinary qualification. The tag workflow still verifies the
complete published catalogue.

Fedora base and donor references use their selected major tag (`:44` or `:45`),
without a digest, because upstream may retire old manifests. This is an explicit
exception to external-image digest pinning; other bases and all published action
images retain digest pins. Renovate must not add Fedora digests back. Tag-based
Fedora builds receive upstream updates within the selected major version, so the
tested OCI archives remain the immutable publication inputs.

Fedora 45 is the standard base for catalogue tooling. The final runtimes of
`lint/rust-gtk`, `workflow/build-rust-gtk` and `workflow/test-rust-gtk` retain Fedora
44 to preserve the consumer's libc/GTK baseline. Release planning and publication
tools do not compile the consumer application and use Fedora 45. Recipes using
Fedora 45 compile their Go entrypoints with the selected Fedora 45 compiler;
upstream tools and published action references retain their own version pins.

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

The three historical activation runs mentioned below were deleted during cleanup
of the temporary workflows. Their IDs are retained as historical context, not
links to available logs or artifacts.

Maintenance controls were subsequently qualified and published in
run `43741811863` (deleted).
Maintenance validates its branch without imposing release completeness. Unpublished
images are explicitly reported as `not_run`, allowing other images, Renovate and
link checks to proceed.

The Atomic fixture exporter and scratch verifier were qualified on both native
platforms in run `43744050295` (deleted).
The fixture retains the real consumer filesystem layers but avoids Buildah VFS
layer snapshots. Only its synthetic source-revision label is changed. The usual
userspace checks and tampered-checksum rejection remain required.

The seven control images below were qualified through real native AMD64/ARM64
Docker-action execution and published without rebuilding in
activation run `43734667929` (deleted),
from source `d54b5b9`. Their verified GHCR index digests are now pinned in the
action metadata. The temporary branch-scoped activation workflow has been removed.
This establishes control-image qualification/publication; normal development and
release orchestration must still be verified separately.

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
