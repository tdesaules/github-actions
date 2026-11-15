# Get OCI artifact

Download an immutable GHCR artifact with ORAS into a new workspace-relative
directory. The caller supplies the expected flat file names and SHA-256 values
from source-bound release evidence. Validate the complete payload set and reject
extra files, directories, symlinks, duplicate names and checksum mismatches.

Public artifacts need no credential. For private artifacts, credentials are
forwarded through stdin/environment into an automatically removed private ORAS
configuration. The static entrypoint and ORAS run in scratch with CA certificates.
No engine socket or host tool is used.

This action verifies transfer integrity, not installer boot or OS functionality.
Follow it with the required functional qualification before release publication.
The source Containerfile reference is bootstrap-only until native qualification
and immutable image publication are complete.
