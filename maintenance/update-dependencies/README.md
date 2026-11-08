# Update dependencies

Action running Renovate 44.181.6 for one explicit GitHub repository.

The runtime includes upstream Zod 4.6.7 and Markdown-it 15.0.3. Undici is replaced
with its locked upstream 8.12.2 release until Renovate incorporates that fix.
Required inputs: `repository` (`owner/name`) and `token`; `dry-run` defaults to
`false`. Supply a repository-scoped installation token generated with the official
App-token action from `vars.BOT_APP_ID` and `secrets.BOT_APP_PRIVATE_KEY`.
Release preparation and maintenance each generate their own token from this shared
bot App. Authentication is passed through the internal `RENOVATE_TOKEN` environment
variable, never an argument; no dedicated Renovate secret or PAT is required.
The App needs contents, pull request and workflow update permissions, plus issues
write access for the Dependency Dashboard. Maintenance uses the App's granted
permissions, while release preparation requests only contents write access.

The scratch runtime includes Node, Git and Go for this repository's dependency
managers. Autodiscovery is disabled; an existing Renovate configuration is
required. It has no host package manager or Docker socket dependency.

The upstream `braces` 3.0.3 AST walkers receive a depth guard for
GHSA-vfj7-8cjw-p6xm. The build verifies the original source hashes and tests the
advisory payload against all three patched walkers. Its original package version
is retained; vulnerability review requires all three patched source hashes in
the SBOM. Missing or modified evidence keeps the vulnerability blocking.

`--repository` and `--dry-run` override `INPUT_REPOSITORY` and `INPUT_DRY-RUN`.
The catalogue workflow invokes this digest-pinned action after its maintenance
App token is generated successfully. Disable hosted Renovate App execution for
the repository to avoid duplicate updates.

## Go tool source upgrades

The runner loads the bundled global `renovate-tool-sources.json` configuration
alongside the repository configuration. It discovers `compile-tool.sh module
version h1:checksum` source pins and runs the fixed
`/usr/local/bin/update-tool-checksums` post-upgrade task to update their Go sums.
Only this command is allowed; public module downloads retain checksum-database
verification. A failed lookup prevents checksum writes. The helper is bundled
with the matching configuration so older published images do not activate an
unsupported post-upgrade hook.

The repository uses `platformCommit: disabled` so Renovate commits the final
working-tree files through Git, including post-upgrade edits of a package file
already updated by a manager. The runner also bundles npm from an exact lockfile
for npm artifact updates. Its bundled Undici is replaced with the locked compatible
6.x fix; the actual package identity and checksum verification are retained.
