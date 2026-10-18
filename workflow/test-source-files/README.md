# Test source files

Repository-internal composite assembling the check catalogue. Provide `checks`
as the planner's JSON object of check names and literal file arrays. Missing
and empty arrays skip their checker. Selected checks continue after another
check fails, while retaining the overall failure result; cancellation stops them.

Requires this repository checked out at the workspace root. The `main` workflow's
`lint` job calls this composite with the planner's affected-file selection.
Every container action used here is pinned to a published immutable digest.
See [migration status](../../docs/orchestration-refactor.md).
