# Build Atomic installer ISO

Build an AMD64 Fedora 45 installer from an exact local OCI archive. The source
checksum is checked before and after construction. The output reports the ISO's
SHA-256 and exact byte size and passes `checkisomd5`.

Installer sources are pinned to build-container-installer v1.5.0 and Lorax
45.4-1, matching the Fedora 45 Lorax RPM. A pre-created branch-45 marker avoids
the old image's empty `git checkout tags/` lookup. Fedora 45 repository files
are made available to the upstream Makefile's explicit repository loader.

## Privilege exception and bootstrap

The Niri Tonic OS user explicitly authorized privileged execution only for
Lorax ISO construction. The native Docker action uses the runner's Docker socket
to inspect only its own image identity and bind mounts, then launches that exact
local image as a privileged worker. It never accepts another worker image or
arbitrary command. Only the workspace, output command file and public run IDs
are forwarded; the worker gets neither the socket nor credentials.
Container inspection is bounded to 30 seconds and worker execution to two hours.
SIGINT/SIGTERM kill the client/tool process groups; worker removal has an
independent 15-second deadline. Cleanup failures are reported even when the
worker itself failed, with its unique name for diagnosis. Uncatchable runner
termination still relies on disposal of the runner.

Inputs and outputs stay inside the workspace; devices and symlinked archives
are rejected. Cancellation cleans up only the uniquely named worker. An isolated
local Podman qualification can invoke the same image with `--privileged` and
`ATOMIC_LORAX_WORKER=1`; that bypasses only the GitHub coordinator, not source
validation. Never pass this environment variable to normal hosted invocations.

Both modes require positive numeric `GITHUB_RUN_ID` and `GITHUB_RUN_ATTEMPT`.
Local fixtures must set explicitly synthetic IDs; they are not hosted run
evidence. The receipt preserves these identifiers and remains `tested: false`.
`iso` and `receipt` outputs are workspace-relative, including when the input
output path was an absolute path inside the workspace.

The ISO and both sidecars are reserved exclusively before transfer. All writes
are confined through an open workspace root, including symlink resolution.
Concurrent sidecar creation never overwrites existing evidence; failed copies,
checksum mismatches and cancellation remove only this invocation's partial
files. The copied ISO bytes are checked against the source ISO checksum.

The runtime includes Lorax 45.4-1, Skopeo, xorriso, loop-device utilities,
`checkisomd5`, and Docker CLI 29.9.3. The Docker CLI LICENSE and NOTICE are
retained with checksum-verified source bytes. Local rootless image builds and
mocked coordinator tests do not qualify Docker CLI 29.9.3 with the hosted
Docker daemon: native `uses:` coordinator execution remains a separate gate.

The source Containerfile reference is bootstrap-only until exact native image
qualification and immutable publication. ARM64 OS images are independent;
ARM64 ISO output is not claimed. Installation and boot evidence are separate
from the constructor's media checksum checks.
