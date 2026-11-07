# Read binary release

Inputs: `tag`, `repository`, `token` (**contents: read**, **actions: read**),
`output` (default `.build/released`, must not already exist).
Nonsecret inputs accept named CLI flags overriding environment/default values,
including explicitly empty values. Tokens are environment-only. Unknown flags and
positional arguments fail without echoing values; `--version` and `--tool-version`
print the action version without performing work.

Outputs: `source_sha`, `version`, `run_id`, `run_attempt`, `qualified_directory`,
`amd64_directory`, `arm64_directory`. Paths retain the caller's workspace-relative
output prefix; each architecture directory contains its original archive/receipt.

Check out the **final tag commit**, with full history and annotated tags, before
calling this action. It verifies the bounded preparation marker, source parent,
and metadata-only Cargo/changelog diff. It checks the source dispatch run and
latest attempt, waits up to eight minutes between API calls for preparation to
finish successfully (individual HTTP calls also have bounded timeouts), paginates
artifact metadata, and downloads only the two bound immutable artifact IDs.
Redirected object downloads never receive the GitHub token. ZIP digest, archive
hash, tested receipts, version/source/run/attempt and both architectures must match.

ZIP extraction permits regular receipts and opaque `.tar.gz` archives only, rejects
traversal/symlinks/duplicate files, and limits expansion to 2 GiB. It never extracts
the binary tarball; `test-rust-gtk` handles that format and executes the exact bytes.
Tag-mode tests should separately check out `source_sha`, use `source: published`
with `source-run-id` and `source-run-attempt` from this action, accept original
tested `*.receipt.json` receipts, and preserve original `run_id`/`run_attempt`. Upload
tag-retested bytes for the publication job. Advisory reports use separate artifacts.

Fedora runtime retains git for local object/diff verification and trust roots for
HTTPS. Temporary `image: Containerfile` metadata must be replaced after native
AMD64/ARM64 qualification and publication. Unit/HTTP tests are in
`shared/binaryrelease`; no real credentials are used by tests.
