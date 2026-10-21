# New release plan

Plans the next stable semantic version using Git history and pinned git-cliff.
Selects the greatest reachable stable SemVer tag and ignores release-only
commits when deciding whether publication is needed. Writes the source revision,
previous version, proposed version and changed flag to a JSON plan.

Inputs: `config` (default `cliff.toml`), `output` (default
`.build/publication/release-plan.json`). Explicit flags override `INPUT_*` values.
Requires full Git history. Does not publish images, mutate refs or create tags.

Its independent scratch image contains Git, git-cliff and a fixed Go executable.
Published with `1.0.1`, it is called directly by `check` for release preparation.
