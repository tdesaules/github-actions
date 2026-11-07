# Publish binary release

Inputs: `repository`, `tag`, `qualified-directory` (default `.build/qualified`),
`token` (**contents: write**, **actions: read**). Output: `tag`.
Nonsecret inputs accept named CLI flags overriding environment/default values,
including explicitly empty values. Tokens are environment-only. Unknown flags and
positional arguments fail without echoing values; `--version` and `--tool-version`
print the action version without performing work.

Run after both tag-mode binary tests succeed. Check out the **final tag commit**
with complete history and annotated tags. Download the complete tag-retested
amd64/arm64 receipts and unchanged archives into `qualified-directory`. Receipts
must preserve the preparation source SHA, version, run ID and run attempt.

The action rechecks the annotated tag and metadata-only source diff, successful
preparation run, immutable artifact IDs/ZIP digests, complete receipts and hashes.
It creates or updates a non-draft, non-prerelease GitHub Release containing the
actual `.tar.gz` binaries, `SHA256SUMS`, and `release-evidence.json`. Uploaded bytes
are streamed with a 2 GiB bound. Existing identical assets are reused; conflicting
bytes are errors and are never deleted/replaced. Asset and release listings are
paginated. Only the newest stable SemVer is explicitly marked latest.

Publication requires selected functional tests to succeed in the consumer workflow;
`tested: true` alone is insufficient authorization. Artifact/source/hash binding
is blocking. Security reports are advisory and do not enter this integrity gate.
Expired preparation artifacts prevent publication/replay; configure retention.

Git object validation requires a Fedora runtime with git and TLS roots.
The entrypoint adopts the `GITHUB_OUTPUT` owner's UID/GID before operations
(workspace owner when unset); version probes require no output file.
Bootstrap `image: Containerfile` is temporary and must be pinned after actual qualification
and publication. `go test ./shared/binaryrelease` exercises repeated publication,
conflicting assets, source/receipt mismatches and pagination using local mocks.
