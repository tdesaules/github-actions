# Test Python

Runs `python3 -B -m unittest discover -s tests/unit -v` in the selected project.
Python, Git and Bash are included for the catalogue's real-Git regression tests.
The fixed Go entrypoint preserves the test process's exit status and cancellation.
Source files may be mounted read-only; temporary fixtures use `/tmp`.

Input `directory` defaults to `.`; `--directory` overrides `INPUT_DIRECTORY`.
`--tool-version` prints the interpreter version. The final stage is `scratch`.
No arbitrary command or operation-selector input is accepted.

The image is awaiting native qualification and immutable publication. The main
workflow retains an explicitly named bootstrap step until publication completes.
