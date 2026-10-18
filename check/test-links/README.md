# Test links

Action using Lychee 0.24.2 in a scratch image. Supply `files` as a
nonempty JSON array of literal paths and optionally `config` as a Lychee TOML
configuration path. Defaults: 30-second timeout, two retries, four concurrent
requests.

Reports per-file results as JSON on stdout and sets `checked` and `passed`.
Any failed file makes the action fail. `--files` and `--config` override
`INPUT_FILES` and `INPUT_CONFIG`. The image is pinned by digest in tag `0.3.0`.
