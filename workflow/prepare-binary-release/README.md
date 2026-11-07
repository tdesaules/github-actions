# Prepare binary release

Dedicated fixed-operation Docker action. Inputs: `source-sha`, `plan` (default
`.build/plan.json`), `qualified-directory` (default `.build/qualified`),
`repository`, `token`, `config` (default `cliff.toml`). Outputs: `tag`, `final_sha`,
`source_sha`, `artifact_run_id`.
Nonsecret inputs also accept named CLI flags; explicit CLI values (including empty
strings) override environment/default values. Tokens remain environment-only;
`--token`, unknown flags and positional arguments are rejected without echoing
values. `--version` and `--tool-version` print the action version.

Run only for a selected main-branch preparation dispatch after both native test
jobs succeed. Check out the immutable original source with full history. Supply a
scoped GitHub App token with **contents: write** and **actions: read**. The App must
be installed on the consumer repository; ordinary workflow tokens do not trigger
the required tag workflow. Secrets are environment inputs, never command arguments.

Each architecture must upload `qualified-<arch>-<github.run_id>` through immutable
Actions upload-artifact v4. Each artifact contains a `.tar.gz` plus `*.receipt.json`:

```json
{"schema_version":1,"source_sha":"<40-hex>","version":"1.0.0","component":"niri-tonic-files","arch":"amd64","archive":"niri-tonic-files-1.0.0-linux-amd64.tar.gz","sha256":"<64-hex>","run_id":"123","run_attempt":"1","tested":true}
```

Other JSON reports in a local download directory are ignored; unexpected `.tar.gz`
files are rejected. Upload only the exact archive and receipt outputs and keep scan
reports in separate artifacts. Receipts retain the exact binary bytes
and source/run/attempt identity. The action verifies both local transfers and the
uploaded immutable artifacts, paginating listings and rejecting expired,
ambiguous, stale-attempt, incomplete, or mismatched artifacts. It binds artifact
IDs, ZIP digests, archive names/hashes and run identity in bounded JSON inside the
annotated `binary-release-v1` tag message. Retain the original Actions artifacts
for the desired release replay lifetime; expired artifacts fail closed.

Only then it updates the root Cargo package/lock version, generates the complete
Keep a Changelog file with git-cliff, makes a single-parent metadata commit with a
why-body, and creates a stable no-v annotated tag. It validates that no other
source or Cargo metadata changed. Main/tag absence is checked before and after
metadata generation. Atomic push plus an exact main lease rejects races; tags
are never force-updated. Checkout token extraheaders are cleared for App-authenticated
pushes. It does not create a GitHub Release.

The binary was built at `source_sha` using `NIRI_TONIC_RELEASE_VERSION`; the tag
points at `final_sha`. This distinction is mandatory for subsequent tests.

Fedora is retained for git/git-cliff and authenticated transport. Source bootstrap
`image: Containerfile` must be replaced with the observed qualified GHCR digest.
Tests live in `shared/binaryrelease`; test fixtures do not push or publish remotely.
