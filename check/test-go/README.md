# Test Go

Runs `go test ./...` in the selected module with a dedicated Go toolchain image.
The fixed Go entrypoint replaces itself with the test process, preserving exit
status and cancellation. Source files may be mounted read-only; caches use `/tmp`.
CGO is disabled. No arbitrary command or operation-selector input is accepted.
Git and its local transport helpers are included for real-repository release
recovery tests. Test fixtures and hooks execute only in the writable temporary
directory, while the checkout can remain read-only.

Input `directory` defaults to `.`; `--directory` overrides `INPUT_DIRECTORY`.
`--tool-version` prints the compiler version. The final image stage is `scratch`.

The image was qualified on AMD64 and ARM64 and published with release `1.0.0`.
The workflow calls this action directly under its changed-file condition.
