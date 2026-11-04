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
and Git LFS 3.8.1 execute on both variants. The ARM64 security rescan and hosted
qualification of this refresh have not been run. The cold Core experiment uses
the unmodified KAS 5.5 Trixie base, before the additional package updates and
Git LFS replacement; its result must not be attributed to the final image.

Trixie is outside the validated host list of the pinned Yocto sources. A container
build and `kas --version` do not qualify a full producer build. These experiments
do not authorize catalogue publication or consumer activation.
