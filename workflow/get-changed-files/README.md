# Get changed files

Reads a Git diff with both paths of renames and deletions preserved. Exposes
literal JSON file lists and Boolean conditions for explicit workflow steps.
Deleted files never become linter inputs. Changes to known linter configurations
select their tracked dependents. Empty/zero base and `full=true` select all files.

Inputs: `before`, `after` (default `HEAD`), `full` (default `false`). Requires a
checkout with the compared history. Explicit flags override `INPUT_*` values.
The action invokes no linters, tests, Python or operation dispatcher.

Its independent scratch image contains Git and a fixed Go executable. Published
with `1.0.1`, it is called directly by `check` before checks and matrix selection.
