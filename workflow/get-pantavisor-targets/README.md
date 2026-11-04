# Get Pantavisor targets

Selects project checks, construction and artifact qualification from the literal
JSON paths produced by `workflow/get-changed-files`. This action does not discover
catalogue images or emit a dynamic matrix. System uses a static base/k0s matrix.

Inputs: `project` (`core`, `k0s`, `system`), `files` (default `[]`), `full`
(default `false`). CLI arguments override `INPUT_*` environment variables.
Outputs are the strings `true` or `false`: `check-required`, `build-required`,
`test-required`. Linter file lists remain owned by `get-changed-files`.

Documentation-only changes need no artifact build. Test-only changes run project
checks. Other inputs, including removals, select construction and qualification
conservatively. Current Pantavisor provenance covers the complete source snapshot;
composition changes cannot reuse an unbound historical build implicitly. Release
callers must set `full: 'true'`, regardless of the tag's diff. Schedule routing is
the caller's responsibility: maintenance does not invoke this selector.

The image has no external tools, network operations or source writes. Its only
file output is the runner-provided `GITHUB_OUTPUT`. Malformed paths and project
kinds fail. Paths with spaces are preserved and deleted paths need not exist.

## Publication status

The explicit `:unpublished` reference is the catalogue's existing bootstrap
convention, not a consumer pin. Build and qualify both native architectures,
publish the image, replace this reference with the verified index digest, then
release the catalogue before activating a Pantavisor consumer.
