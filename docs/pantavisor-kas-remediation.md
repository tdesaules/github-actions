# KAS runtime security refresh

## Hosted failure and remediation

Run [41797577398](https://github.com/tdesaules/github-actions/actions/runs/41797577398)
built and executed the KAS action on native AMD64 and ARM64. Both variants failed
the `high` vulnerability gate. The original AMD64 report contained 108 critical
and 646 high matches; these are package/advisory matches, not unique CVEs.

The refresh uses the verified multi-platform KAS 5.5 Debian Trixie release index,
explicitly versioned Debian security updates, and Git LFS 3.8.1 rebuilt with
Go 1.27.3 and corrected x/crypto/x/net dependencies. Package identities, licenses
and the `high` gate remain intact. No blanket scanner exclusions are introduced.

The runtime collector resolves virtual package providers (`pkg-config` is
provided by `pkgconf`) and selects the interpreter's Python library directory.
The UTF-8 locale is regenerated after updating libc/locales. Git LFS is replaced
as a binary and its obsolete distro package is not included in the runtime's
package database.

## Remaining upstream blockers

Checked against the Debian Security Tracker on 2026-11-04:

| Component | Example advisory | Stable status |
| --- | --- | --- |
| GCC/libstdc++ | [CVE-2026-102010](https://security-tracker.debian.org/tracker/CVE-2026-102010) | Trixie 14.2.0-19 vulnerable; fixed in unstable 14.4.0-3 |
| GCC/libstdc++ | [CVE-2026-95619](https://security-tracker.debian.org/tracker/CVE-2026-95619) | Trixie 14.2.0-19 vulnerable; fixed in unstable 14.4.0-3 |
| glibc | [CVE-2026-19499](https://security-tracker.debian.org/tracker/CVE-2026-19499) | Trixie 2.41-12+deb13u4 vulnerable; fixed in unstable 2.43-5 |
| libcurl | [CVE-2026-10536](https://security-tracker.debian.org/tracker/CVE-2026-10536) | Trixie security 8.14.1-2+deb13u7 vulnerable; fixed upstream in 8.21.0 |

Debian classifies these examples as minor/no-DSA or postponed. That classification
does not prove the code is absent or unreachable in this build action and does
not automatically waive the catalogue's scanner policy. Moving the entire
toolchain to unstable or backporting these fixes requires separate construction,
compatibility and vulnerability evidence. A rerun of the same image is not a fix.

Some binary/Python matches also need comparison with distro backports. The
presence of such duplicate identities does not establish that every remaining
alert is a false positive.

## Evidence

Local experimental reports are retained in ignored directories:

- `.build/kas-refresh/`: KAS 5.5 Bookworm comparison.
- `.build/kas-trixie/`: Trixie release image and cold Core compatibility run.
- `.build/kas-trixie-updated/`: Debian package updates before Git LFS replacement.
- `.build/kas-remediation/`: replacement Git LFS and UTF-8 locale checks.
- `.build/kas-remediation-v2/`: additional x/net correction and rescan.

The final AMD64 rescan reports 47 critical and 366 high matches (413 matches,
145 distinct advisory identifiers), down from 754 blocking matches initially.
No blocking Go-module matches remain. Git LFS version, clean-filter/pointer
creation and `git lfs fsck` pass in the refreshed image; the UTF-8 locale also
loads successfully. These checks do not turn the remaining scan failure into
a successful qualification.

The final image builds on AMD64 and on ARM64 under local emulation. KAS 5.5
and Git LFS 3.8.1 execute on both variants. The final ARM64 scan reports 47
critical and 308 high matches (355 matches, 145 distinct advisory identifiers),
down from 666 initially. No blocking Go-module matches remain on ARM64 either.
Its reports are in `.build/kas-remediation-v2/arm64/`. Hosted qualification of
this refresh has not been run.

The intermediate Trixie image completed a cold Core build in 2540 seconds.
The final corrected image then completed a separate cold Core build in 2461
seconds, with `cold: true` and `success: true` recorded in
`.build/kas-remediation-v2/core-cold/source/artifacts/ci/build-report.json`.
Packaging, component integrity verification, OCI assembly and ARM64 smoke tests
for Bash 5.3.20, kubectl 1.36.4, k0sctl 0.33.1 and k9s 0.51.0 passed on its outputs.
This is local AMD64-hosted construction of ARM64 Core, not an ARM64-hosted Yocto
build, a k0s/System build or a hosted GitHub producer qualification.

Trixie is outside the validated host list of the pinned Yocto sources, despite
the successful local Core build. The remaining vulnerability failures still
prevent catalogue publication and consumer activation.

## File ownership correction and remaining priorities

The trimmed runtime originally retained `/var/lib/dpkg/status` but omitted the
selected packages' `.list` and `.md5sums` files. Consequently, Syft could not link
some upstream binary/Python identities to their owning Debian packages. The
collector now retains those upstream ownership/checksum records too.

The AMD64 comparison in `.build/kas-ownership/` shows:

- 10,925 common files with recorded digests have identical contents before and
  after the metadata correction; no executable or library was changed.
- Syft emits 20 `ownership-by-file-overlap` relationships.
- Grype's normal distro ownership handling removes generic blocking matches for
  OpenSSL, gzip and GitPython, with no new ignore configuration.
- The blocking result is now 31 critical and 270 high matches, representing 76
  distinct advisory IDs. This is still a failed scan. These new counts have not
  yet been verified on ARM64 or GitHub.

The Debian tracker confirms, for example, that OpenSSL CVE-2026-75803 is fixed
in `3.5.7-1~deb13u2` and gzip CVE-2026-41992 in `1.13-1+deb13u1`, both covered by
the installed versions. Ownership records allow the scanner to use those distro
versions instead of only upstream version strings.

The next actual remediation priorities are:

| Dependency | Remaining distinct advisories per package | Approach |
| --- | ---: | --- |
| curl / both libcurl variants | 31 | Backport upstream fixes or rebuild newer libcurl; preserve both TLS backend variants and check Git/KAS downloads |
| libxml2 | 8 | Backport relevant fixes; a newer major library requires ABI checks for its existing consumers |
| Python 3.13 | 5 | Apply upstream security patches or update within 3.13, then verify KAS and BitBake |
| Expat | 4 | Backport parser fixes, including the Python integration requirements |
| SQLite | 3 | Backport upstream fixes and test its Python consumer |
| Wget | 3 | Upstream patches exist even though the distro report has no fixed package version; rebuild and test downloads |
| glibc / GCC | 3 / 2 | Targeted backports and toolchain validation; avoid mixing unstable binary packages into the stable runtime |

The counts above overlap across binary packages and are not additive. Other
remaining components include gawk, ACL, Kerberos, SASL, GnuPG, ncurses, Perl,
util-linux and zlib. Tracker status `open`/`no-dsa` is not evidence of
non-applicability. Updating package metadata resolves the scanner's ownership
problem, not these remaining upstream vulnerabilities.

## curl source backport

`build-curl.sh` builds the curl 8.23.0 upstream tarball with the Debian 8.23.0-1
packaging, both pinned by SHA-256. It uses Trixie build dependencies and official
backports for nghttp3/ngtcp2, not sid binary packages. Its locally built packages
are identified as `8.23.0-1~pantavisor1`. Debian's GnuTLS compatibility patch
preserves the symbols required by the existing Git binary. HTTP/3 remains enabled.

The AMD64 and locally emulated ARM64 fixtures pass both backend version checks, HTTPS redirects, rejection
of untrusted certificates and hostname mismatches, Git HTTPS cloning through
GnuTLS, and KAS checkout of an exact commit through the same local TLS server.
The fixture CA is trusted only inside disposable test containers. Logs and scans
are under `.build/kas-curl/`. This is not another full cold Yocto qualification.
The curl-update scan was run on AMD64 only; ARM64's final rescan and the hosted
qualification of this backport remain to be performed.

The raw Grype result does **not** decrease: it reports 39 critical and 293 high
matches, including the same 31 curl CVEs against all four Debian binary packages
(the new GnuTLS package plus its transitional compatibility package increase the
match count). The Trixie feed marks these CVEs open with no upper affected-version
bound, even for the newly built 8.23.0 source version. The corresponding Debian
sid records resolve all 31 at source versions up to 8.23.0-1; the comparison is
recorded in `.build/kas-curl/advisory-comparison.json`.

No blanket curl exclusion or distro override is applied. The raw scan still
fails. A narrowly scoped, evidence-verified review of this source backport is
needed before the scanner can distinguish it from the affected Trixie packages.
Other components' findings remain independent of this curl update.
