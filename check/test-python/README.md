# Test Python

Runs `python3 -B -m unittest discover -s tests/unit -v` in the selected project.
Python, Git and Bash are included for the catalogue's real-Git regression tests.
The fixed Go entrypoint preserves the test process's exit status and cancellation.
Source files may be mounted read-only; temporary fixtures use `/tmp`.

Input `directory` defaults to `.`; `--directory` overrides `INPUT_DIRECTORY`.
`--tool-version` prints the interpreter version. The final stage is `scratch`.
No arbitrary command or operation-selector input is accepted.

The image was qualified on AMD64 and ARM64 and published with release `1.0.0`.
The workflow calls this action directly under its changed-file condition.

## Candidate extension for Pantavisor

The source entrypoint additionally accepts `--tests-directory` (or
`INPUT_TESTS-DIRECTORY`) to discover tests under a project-relative directory,
including `tests`. The default remains `tests/unit`. Explicit CLI input takes
precedence; empty, missing, absolute and escaping paths fail. Test failures and
cancellation keep the same process exit behavior.

The source metadata declares the optional `tests-directory` input with its legacy
default and forwards it through the environment. It must only be consumed after
the candidate image is qualified on both architectures and published in a new
catalogue tag. The current published image reference preserves default callers;
editing metadata alone does not add the extension to that existing binary.
