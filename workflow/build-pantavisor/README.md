# Build Pantavisor

Builds `kas/ci.yml` for `core`, `k0s` or `system` inside the action, without a
container engine, socket, privileged mode or host sysctl changes. KAS 5.5 and
the Debian Trixie build-tool runtime are extracted from the pinned upstream
multi-platform image, with explicit Debian security package updates. Compilers
are required runtime dependencies of this build action.

Git LFS 3.8.1 is built from checksum-verified upstream source with the catalogue's
Go toolchain and explicit x/crypto and x/net fixes. The distro Git LFS package is replaced,
not copied alongside it. Its module identity and license remain in the image.
Update its source version and checksum together. The collector resolves virtual
package providers and follows the selected Python interpreter's library version.
The UTF-8 locale is regenerated after the libc/locales updates.

Additional source backports preserve Debian package ownership and retain only
the runtime dependency closure: Python 3.13.18, libxml2 2.15.4 with its rebuilt
Perl binding, gawk 5.4.1, SQLite 3.54.1, Expat 2.10.0, ACL, zlib, ncurses,
SASL, Kerberos and GnuPG. The two affected Perl core modules are replaced in
place using pinned CPAN releases without changing the interpreter ABI. GnuPG's
matching meta/localization packages are included to retain `python3-gnupg`.

The gawk persistent-memory allocator (PMA) is disabled because its process
personality changes are unavailable in the Docker sandbox. Its enabled upstream
tests run as a non-root user. SQLite's permission-sensitive tests also run
non-root. Wget receives bounded-size fixes and a pinned upstream filename
conversion fix, with source-level bounds regressions in its build stage.

See [the remediation evidence](../../docs/pantavisor-kas-remediation.md) for
local test coverage and the remaining publication gates. Native ARM64 hosted
qualification is required; AMD64 results do not establish ARM64 qualification.

curl 8.23.0 is rebuilt from checksum-pinned upstream and Debian packaging sources
against Trixie. Both OpenSSL and GnuTLS variants retain Debian's symbol versions;
HTTP/3 dependencies are pinned to official Trixie backports. The resulting local
Debian packages retain their real source version and file ownership. No unstable
binary repository is enabled. `scripts/test-pantavisor-curl.py` exercises HTTPS,
certificate and hostname rejection, the Git GnuTLS ABI and KAS repository checkout.
The Debian package build uses `nocheck`; these focused integration tests do not
claim that the full upstream curl test suite has run.

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
