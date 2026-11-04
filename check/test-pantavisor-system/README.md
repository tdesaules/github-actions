# Test Pantavisor System

Runs the fixed `python3 -B -m tools.pvbuild check` command from the selected
`directory` (default `.`). The checked-out System project owns the checks. This
image supplies Python, Git, Bash, GNU cpio and the GRUB syntax/environment tools.
It does not build Yocto or run a VM.

`--directory` overrides `INPUT_DIRECTORY`. `--tool-version` prints the Python
version. The Go entrypoint replaces itself with Python, preserving exit status
and signal delivery. Git trusts only the selected project directory via temporary
environment configuration, preserving existing environment entries.

The checkout must be writable: the existing System check explicitly compiles
Python bytecode and creates its ignored artifacts directory. No board or registry
credentials are required. Container root permits those writes on Docker runner
mounts; tests also exercise execution as the workspace owner locally.

The explicit `:unpublished` reference is bootstrap metadata. Publish and qualify
both native architectures and replace it with the verified digest before any
consumer invokes this action through a released catalogue tag.
