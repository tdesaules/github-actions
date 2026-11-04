# Build Pantavisor

Builds `kas/ci.yml` for `core`, `k0s` or `system` inside the action, without a
container engine, socket, privileged mode or host sysctl changes. KAS 4.8.2 and
the build-tool runtime are extracted from the pinned upstream image. Compilers
are required runtime dependencies of this build action.

Runs non-root as the checkout owner, with `/work` pointing to that checkout.
Existing build/staged-output directories are refused by default. `cold: 'false'`
allows explicit diagnostic recovery of a build tree, never overwriting a staged
export; the report records that it was not a cold build. The consumer workflows
keep the cold default. Producer provenance binds
the source before construction to the deploy files after success. Deploy files
are staged under `artifacts/ci/deploy` before this invocation's build tree is
removed. `artifacts/ci/build-report.json` records duration, status and free disk.

The package database is retained for vulnerability scanning. The CI overlay must
limit concurrency and discard compressed sstate only after relocation hooks.
The hosted Docker sandbox and full cold builds must pass before release. The
bootstrap `:unpublished` image is not a consumer reference.
