# Test Markdown

Candidate action using markdownlint 0.41.1 in a scratch image. Supply `files`
as a nonempty JSON array of literal Markdown paths. No glob expansion occurs.
The workspace-root `.markdownlint.json` configuration is honored. Only the
markdownlint library and its runtime dependency closure are included; CLI glob
matching, fix mode and custom CLI formatters are not part of this action.

Reports per-file results as JSON on stdout and sets `checked` and `passed`.
Any failed file makes the action fail. `--files` overrides `INPUT_FILES`.
The image reference remains unpublished.
