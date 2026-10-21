# Workflow orchestration

The `lint` job calls each linter directly: Actionlint, Hadolint, ShellCheck,
YAML, TOML, Markdownlint and Lychee. Each action lives under `lint/<linter>/`
with its own `action.yml`, `Containerfile`, Go entrypoint and OCI image.

The `check` job computes the Git diff immediately after checkout, before testing.
It exposes the changed file lists and Boolean outputs used by each linter's
`if` condition. There is no generic lint planner or dispatcher action.
Renames include both paths; deleted files are excluded from linter inputs.
First pushes without a base, manual dispatches and tags check all tracked files.
Changes to a linter configuration check all files affected by that configuration.

Documentation changes select Markdownlint and Lychee. An isolated Containerfile
change selects Hadolint. YAML workflows also select Actionlint. Action metadata
validation is performed in `check`, separately from the linters.

The same diff selects catalogue, Go and Python checks. Documentation-only changes
skip all three suites. Go changes select Go tests; Python scripts and unit-test
changes select Python tests. Metadata and Containerfiles select catalogue and
Python relationship regressions. Workflow changes and full runs select all checks.
Deleted and renamed paths participate in these conditions.

`check/test-catalog`, `check/test-go` and `check/test-python` each own their
Containerfile, fixed Go entrypoint and image. Their published `1.0.0` images are
called directly; the bootstrap commands and `*-published` outputs are removed.
Runner qualification also assembles published fixture and Buildah actions.

Four further independent Go actions now perform the orchestration in `check`:
`workflow/get-changed-files`, `workflow/get-image-matrix`,
`workflow/new-release-plan` and `workflow/test-artifact-source`. Their immutable
images were published with `1.0.1` by run `39621479547`. They are called directly
with explicit inputs and outputs. They do not dispatch linters or tests; those
remain explicit steps in the workflow. Neither `check` nor `lint` contains a
`run` step. Checkout and artifact transport use the official GitHub actions.

Source-check selection is separate from release matrix selection: release planning
and exact-source artifact validation still precede the image matrix calculation.

The published container references remain pinned to the independently qualified
images from `0.3.0`. Moving the source directories does not fabricate new registry
digests or rename published packages. Subsequent qualification builds each image
from its new source location.

## Remaining orchestration work

The release migration introduces separate native actions for publication planning,
archive/index publication, signing, signature/tag verification, atomic release
finalization and GitHub Release creation. The native build matrix qualifies each
candidate before the release job switches to its published immutable image.
Host publication remains in place during this image qualification phase.

Maintenance link checks now call the published changed-files and Lychee actions.
The obsolete shared-package retirement input and deletion steps are removed.
Two native candidates provide catalogue SBOM generation and vulnerability scans;
the existing scan call remains until both are published. The Go advisory-review
implementation preserves the prior fail-closed evidence checks and `high` gate.

Renovate is wired through `maintenance/update-dependencies` with the dedicated
`RENOVATE_TOKEN` secret. Set the repository variable `RENOVATE_ENABLED=true` only
after configuring that token and disabling the hosted Renovate App for this
repository. The variable is currently unset and the secret is absent; bot updates
therefore remain inactive. Release App credentials are not reused for Renovate.

Artifact aggregation, release planning/finalization and registry signature
fixtures still use host orchestration. They require separate reusable interfaces.
The full release and maintenance activation will follow successful candidate
qualification and publication.
