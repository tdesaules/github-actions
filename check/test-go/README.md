# Test Go

Runs `go test ./...` in the selected module with a dedicated Go toolchain image.
The fixed Go entrypoint replaces itself with the test process, preserving exit
status and cancellation. Source files may be mounted read-only; caches use `/tmp`.
CGO is disabled. No arbitrary command or operation-selector input is accepted.

Input `directory` defaults to `.`; `--directory` overrides `INPUT_DIRECTORY`.
`--tool-version` prints the compiler version. The final image stage is `scratch`.

The image is awaiting native qualification and immutable publication. The main
workflow retains an explicitly named bootstrap step until publication completes.
