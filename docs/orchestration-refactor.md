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

Four further independent Go actions replace the remaining Python orchestration:
`workflow/get-changed-files`, `workflow/get-image-matrix`,
`workflow/new-release-plan` and `workflow/test-artifact-source`. They are first
qualified as native candidates by the build matrix. Their calls in `check` will
replace the remaining host commands after immutable image publication. They do
not dispatch linters or tests; those remain explicit steps in the workflow.

Source-check selection is separate from release matrix selection: release planning
and exact-source artifact validation still precede the image matrix calculation.

The published container references remain pinned to the independently qualified
images from `0.3.0`. Moving the source directories does not fabricate new registry
digests or rename published packages. Subsequent qualification builds each image
from its new source location.

## Remaining orchestration work

Artifact aggregation, release planning/finalization and registry signature
fixtures still use host orchestration. They require separate reusable interfaces.
Renovate's image is published, but its maintenance activation still requires a
dedicated bot/App token. Release credentials are not implicitly reused.
