# Build Pantavisor

Builds `kas/ci.yml` for `core`, `k0s` or `system` inside the action, without a
container engine, socket, privileged mode or host sysctl changes. KAS 5.5 and
the Debian Trixie build-tool runtime are extracted from the pinned upstream
multi-platform `latest-debian-trixie` snapshot, pinned by digest rather than
following the mutable tag, with explicit Debian security package updates. Compilers
are required runtime dependencies of this build action.

Git LFS 3.8.1 is built from checksum-verified upstream source with the catalogue's
Go toolchain and explicit x/crypto and x/net fixes. The distro Git LFS package is replaced,
not copied alongside it. Its module identity and license remain in the image.
Update its source version and checksum together. The collector resolves virtual
package providers and follows the selected Python interpreter's library version.
The UTF-8 locale is regenerated after the libc/locales updates.

System compilers and libraries are supplied by Debian, not rebuilt in this
action. The collector retains only the required package dependency closure,
including ownership/checksum metadata so scans recognize distribution backports.
Historical source-backport scripts remain in the repository but are not invoked
by the current Containerfile. Their exact-version/file-hash review rules do not
apply to unmodified distribution packages.

See [the remediation evidence](../../docs/pantavisor-kas-remediation.md) for
local test coverage and the remaining publication gates. Native ARM64 hosted
qualification is required; AMD64 results do not establish ARM64 qualification.

curl 8.21.0 and its HTTP/3 dependencies use explicitly versioned, signed Debian
Trixie backports packages. Both OpenSSL and GnuTLS variants are installed, with
the compatibility package needed by Git. No unstable binary repository is
enabled. `scripts/test-pantavisor-curl.py --curl-version 8.21.0` exercises HTTP/3
feature retention, HTTPS, certificate and hostname rejection, the Git GnuTLS ABI
and KAS repository checkout (supply `--reference` for the candidate image).

The security refresh is not a clean-scan claim. Debian stable still has open
advisories, including GCC's
[CVE-2026-102010](https://security-tracker.debian.org/tracker/CVE-2026-102010).
The `high` gate remains blocking; no blanket exclusions are added for toolchains,
distribution packages or missing fixes. Trixie is also outside the validated host
list of the currently pinned Yocto sources, so its compatibility requires actual
build evidence rather than just a successful KAS version command.

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
