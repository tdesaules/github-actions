# Get execution plan

The image is published and pinned by digest in tag `0.3.0`.

Inputs: `before` (base commit), `after` (checked-out target commit), `full`
(default `false`). Push events supply the base commit automatically. Missing
bases fall back to the merge-base with `origin/main`, or full selection when no
distinct base exists. Fetch history before invoking the action.

Outputs include `plan`, `matrix`, `checks`, `actions`, `has-work`, `go-packages`
and `orchestration-tests`. The matrix has native AMD64/ARM64 runners and a
`rebuild` flag. An empty selection produces `{"include":[]}`.

The CLI accepts the same named flags, plus `--repository`. Explicit flags take
precedence over `INPUT_BEFORE`, `INPUT_AFTER` and `INPUT_FULL`. See the
[migration status](../../docs/orchestration-refactor.md).
