# Linters

Each directory owns an independent Docker action, Containerfile and fixed Go
entrypoint. The workflow invokes these actions directly with literal JSON file
lists; its `if` conditions use the Git diff calculated in the `check` job.

| Directory | Tool |
| --- | --- |
| `actionlint` | GitHub workflow validation |
| `hadolint` | Containerfile/Dockerfile lint |
| `shellcheck` | Shell script lint |
| `yaml` | YAML syntax and duplicate keys |
| `toml` | TOML syntax and duplicate keys |
| `markdownlint` | Markdown style |
| `lychee` | Links |

Each action references its own published OCI image by immutable digest. Existing
registry package names retain the `check-*` prefix from their publication in
`0.3.0`; source paths are now `lint/<linter>/`.
