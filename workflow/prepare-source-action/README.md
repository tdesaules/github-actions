# Prepare source action

Temporary composition of qualified OCI actions for policy-transition tooling.
It builds from the checkout, records and verifies the archive/source/platform,
executes native runtime checks, and generates an action pinned to the isolated
registry digest. It never publishes to GHCR or changes source action metadata.

Callers invoke the generated action explicitly; this composite does not execute
arbitrary business operations. The registry is a job-local service, not an engine
socket. Source bootstrap is functional for catalogue validation/publication;
security-only bootstrap failures remain advisory.

Remove each call after the corresponding policy-aware action is published and
its real digest is pinned. Delete this bridge when all calls are gone. Candidate
images are not a released catalogue and do not activate Pantavisor consumers.
