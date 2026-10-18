# Import candidate image

Candidate action transferring an OCI archive without a Docker socket or host
engine command. Requires a private workflow service named `candidate-registry`
listening on container port 5000 and exposed to the host runner.

Inputs: `archive`, `repository` (simple name), `host-port` (default 5000).
Outputs: `digest` and the host-loopback `image` reference pinned to that digest.
Pass `image` to `workflow/new-candidate-action` for a native Docker action test.

The transfer preserves digests. HTTP is permitted only for the fixed internal
service address; this operation cannot publish to an arbitrary external registry.
Public image publication retains its TLS verification requirements.

The image is not published and the service path still needs hosted qualification.
