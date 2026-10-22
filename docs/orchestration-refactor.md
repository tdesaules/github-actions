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

Moving the linter source directories initially preserved the qualified `0.3.0`
image references. Subsequent releases updated those immutable digests after native
qualification, without renaming published packages.

## Release and maintenance

The release job calls separate native actions for publication planning,
archive/index publication, signing, signature/tag verification, atomic release
finalization and GitHub Release creation. These images were qualified by run
`39730651196` and published without rebuilding by run `39794449722` in `1.1.0`.
The release job contains no host command steps. Releases always require signatures;
the historical workflow bootstrap and optional-signing inputs are retired.

Maintenance link checks now call the published changed-files and Lychee actions.
The obsolete shared-package retirement input and deletion steps are removed.
Published native actions generate catalogue SBOMs and scan both architectures.
The maintenance job contains no host command steps. The Go advisory-review
implementation preserves the prior fail-closed evidence checks and `high` gate.

Renovate is wired through `maintenance/update-dependencies` with the dedicated
`RENOVATE_TOKEN` secret. Set the repository variable `RENOVATE_ENABLED=true` only
after configuring that token and disabling the hosted Renovate App for this
repository. The variable is currently unset and the secret is absent; bot updates
therefore remain inactive. Release App credentials are not reused for Renovate.

## Remaining orchestration work

The `build` job still uses host orchestration for construction, candidate loading,
fixtures and reports. Its conversion remains separate from the four migrated jobs
(`check`, `lint`, `release`, `maintenance`).
