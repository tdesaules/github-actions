# Test Markdown

Candidate action using markdownlint-cli2 0.23.3 in a scratch image. Supply `files`
as a nonempty JSON array of literal Markdown paths. No glob expansion occurs.
The repository's markdownlint configuration is honored.

Reports per-file results as JSON on stdout and sets `checked` and `passed`.
Any failed file makes the action fail. `--files` overrides `INPUT_FILES`.
The image reference remains unpublished.
