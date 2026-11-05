# KAS runtime security refresh

## Current strategy (2026-11-05)

Use Siemens' recent `latest-debian-trixie` image pinned to multi-platform digest
`sha256:0f929318babc0291ab940d6d6db535906dd32e4cf6a0914f0cd19b42ae7453f9`.
The tag describes the upstream channel; the digest fixes the actual input.
Retain explicitly versioned Debian security updates and the minimal package
closure, including package ownership metadata. The action executable and the
already-qualified Git LFS binary are still built with the pinned Go toolchain.
curl and both TLS libraries now use official security revision
`8.14.1-2+deb13u7`. The user selected that stream after source inspection confirmed
its CVE-2026-92392 fix is missing from the newer 8.21.0 backport. The backports
repository and HTTP/3 dependency overrides are removed. See the
[critical exposure assessment](pantavisor-kas-critical-exposure.md).
The inspected repositories did not offer newer applicable glibc, GCC, Python,
libxml2, Expat, util-linux or Wget packages for this baseline.

The stable-security image passes both architecture runtime and HTTPS/Git/KAS
fixtures locally (ARM64 under emulation). Its native curl/TLS bytes match exact
Debian binary packages downloaded through authenticated APT indexes. The seven
applicability witness sets are unchanged; their whole-image fingerprints were
revalidated after reviewing all 50 changed paths per architecture, limited to
curl/dependencies, their package records and the generated loader/package caches.
The collector now explicitly creates empty `/github/workspace` directories so
local layered and hosted squashed exports have consistent inventories.

Current reviewed results are **30 critical / 215 high on AMD64** and
**30 critical / 180 high on ARM64**, covering **70 blocking IDs**. The eight
8.21.0-specific fixed-advisory reviews do not apply to this version. This increase
is the disclosed tradeoff for the stable security correction, not a lowered
threshold or new risk acceptance. A fresh cold Core build is running for this
exact image; hosted qualification is still required.

System-library and compiler source backports are **disabled** in the current
Containerfile. Their committed scripts and evidence remain available for a
future narrowly justified correction. The experimental, uncommitted GCC/glibc
builders have been removed and the long-running GCC build was stopped. Neither
experimental GCC nor glibc was installed in a published action.

Scanning the exact unmodified Siemens image without exclusions reports:

| Architecture | Critical matches | High matches | Distinct high/critical IDs |
| --- | ---: | ---: | ---: |
| AMD64 | 38 | 462 | 189 |
| ARM64 | 38 | 404 | 189 |

These upstream counts are not the final action's scan. Reports and exact source
references are under `/tmp/opencode/kas-latest-comparison/`. The reduced action's
new reports are under `/tmp/opencode/kas-upstream-action/`. Historical backport
reviews require exact versions and payload hashes; they must not waive the
distribution packages now selected. The `high` gate remains blocking.

Before advisory triage, the reduced action with the official curl 8.21.0 backport reports:

| Architecture | Critical matches | High matches | Distinct high/critical IDs |
| --- | ---: | ---: | ---: |
| AMD64 | 39 | 294 | 77 |
| ARM64 | 39 | 236 | 77 |

Compared with the reduced AMD64 baseline before the curl update, the only new
blocking package/advisory pairs are the same 31 curl advisories attached to the
additional `libcurl4-gnutls` package. No new advisory ID appeared. This is not
evidence that every curl finding is applicable or fixed; their individual
upstream/backport status still needs review. At that baseline, evaluation of the
historical policy against both SBOMs emitted **zero exclusions**.

The [hosted-run advisory triage](pantavisor-kas-triage.md) subsequently verifies
eight curl advisories fixed by the installed official 8.21.0 packages. Their
finite, native-file-hash-bound review reduces the local rescans of the hosted
SBOMs to **23 critical / 278 high on AMD64**, and **23 critical / 220 high on
ARM64**, covering **69 distinct blocking IDs**. These are recognized upstream
fixes, not risk acceptances. All remaining findings still block publication.

On the former backports baseline, applicability verification against run `42046121374` establishes
seven further non-applicable IDs, with exact native bytes and independent OCI
inventory checks. Local rescans now have **22 critical / 222 high on AMD64** and
**22 critical / 187 high on ARM64**, covering **62 distinct blocking IDs**. These
bounded proofs expire at 2026-12-05 00:00 UTC. Hosted run `42098220154` subsequently
confirmed those results. The same seven semantic proofs were revalidated for
the current stable-security tree; their package/file witnesses and expiry were
not relaxed. See the triage report for their conditions and the unresolved
util-linux candidate.

### Remaining advisory triage

No risk-acceptance exclusion has been approved or added. For each remaining
advisory, record the exact image/package version, upstream status, affected API,
reachable caller and trust boundary before deciding whether an exception is
justified. An image being a CI build tool is not sufficient evidence by itself.

| Area | Evidence still needed | Current decision |
| --- | --- | --- |
| glibc formatting/gettext | Actual callers and whether format, catalogue path or environment can cross a trust boundary | Keep findings blocking |
| GCC/libstdc++ | Affected ABI/configuration and installed header consumers | Keep findings blocking |
| util-linux | Shipped commands/libraries, privileges and namespace/mount use | Keep findings blocking |
| Download/archive/XML/Python tools | Input provenance, verification order and affected parsing paths | Keep findings blocking |

Any future risk acceptance must name its owner, finite advisory/package scope,
supporting evidence, expiration and upstream-remediation removal condition.
Expiry must be enforced by the review implementation before such a rule can be
used in CI. The existing immutable backport/symbol reviews are not a general
risk-acceptance mechanism. Prefer distribution updates; reintroduce a local
backport only when necessary and qualify its compatibility on both architectures.

### Compatibility validation

The first distribution-based AMD64 action builds and passes Python extension
smoke checks plus HTTPS redirects, certificate/hostname rejection, Git/GnuTLS
cloning and KAS checkout. Its pre-backports scan has 31 critical and 271 high
matches across 77 distinct blocking IDs, with no historical review rule applied.
The subsequent official curl 8.21.0 backport restores HTTP/3 support without
local compilation. Use `scripts/test-pantavisor-curl.py --curl-version 8.21.0`
with the candidate `--reference` to qualify that image. Both final variants pass
the runtime/negative-fixture harness and the HTTPS/Git/KAS fixture; ARM64 was
executed under local emulation. The historical
Python 3.13.18 regression fixture is not evidence for distribution Python 3.13.5.
The distribution-based AMD64 action completed a fresh Core build in
**2439.224 seconds**, with `cold: true` and `success: true`. Its staged outputs
include squashfs, tar, manifest, SPDX, the Core release description and bound
build provenance. Evidence is retained in
`.build/kas-upstream-core-cold/source/artifacts/ci/` and the adjacent build log.
An earlier attempt in `/tmp` stopped on BitBake's low-inode guard after 497
successful tasks; the successful run used a separate fresh checkout on the
repository filesystem, without disabling any disk or inode guard.

This qualifies a local AMD64-hosted Core build, not a k0s/System build or a
native ARM64-hosted producer build. Hosted qualification and resolution of the
security gate are still required before publication or consumer activation.

## Historical source-backport experiments (superseded)

An earlier integrated AMD64 scan, before the final Wget package
is added, has **no critical matches and 143 high matches across 12 distinct
advisories** after evidence-verified reviews. Remaining groups are glibc (3),
GCC/libstdc++ (2), util-linux (4), and Wget (3). The CI gate is not cleared.

The staged source refresh includes Python, XML/Perl, gawk, SQLite, Expat, ACL,
zlib, ncurses, SASL, Kerberos, GnuPG, and two Perl core-module fixes. Relevant
evidence is retained in `.build/kas-python/`, `.build/kas-xml/`,
`.build/kas-gawk/`, `.build/kas-sqlite/`, `.build/kas-expat/`,
`.build/kas-small-libraries/`, `.build/kas-auth-libraries/`, `.build/kas-perl/`,
`.build/kas-integrated/`, and `.build/kas-wget/`.

- Core built cold with Python 3.13.18 in 2802.542 seconds; that experiment does
  not include every later support-library update.
- The rebuilt XML Perl binding passed 2577 upstream tests. Its runtime parses
  Texinfo's XML output, and the legacy libxml2 ABI is absent from the closure.
- SQLite's upstream suite completed 979204 tests with zero errors under the
  non-root build identity.
- gawk 5.4.1 passed its enabled upstream tests with PMA disabled for the action
  sandbox; the `awk` alternative and substitution behavior were checked.
- HTTPS redirects, certificate/hostname rejection, Git's GnuTLS clone path and
  KAS checkout pass in the assembled runtime.
- The Python missing-hostname TLS regression rejects the formerly accepted
  configuration. SQLite, Expat, zlib, XML, CPAN module versions and the GnuPG
  Python binding load correctly in the assembled runtime.
- The Python-only ARM64 image also builds under emulation and passes its version,
  extension ABI, missing-hostname TLS, HTTPS, Git and KAS checkout checks. This
  does not qualify the later complete dependency set on ARM64.
- GnuPG signing, verification and rejection of a modified message pass with an
  ephemeral key in a disposable container.
- Wget's source package build and bounds regressions pass on AMD64. Its final
  image integration, architecture-specific evidence and review remain to be
  qualified; no Wget exception has been added to the policy.

The original separate-branch reviewer rollout was superseded by consolidation
on `main` and the same-run candidate bootstrap described below. The following
sections retain historical measurements; they do not qualify the current
distribution-based image.

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

## Evidence-verified review rollout

The new optional `verified_debian_backports` policy scopes curl's 31 reviewed
advisories to the exact four local Debian package versions and SHA-256 hashes of
the curl executable plus both TLS libraries, separately for AMD64 and ARM64.
The reviewer emits no rules for old versions, modified/missing bytes, duplicate
evidence, missing packages, mixed architectures or a different distro. The source
archive hashes and per-advisory references are retained in the evidence report.

The local AMD64 scan with this verified review reports 7 critical and 201 high
matches, representing 45 distinct remaining advisory IDs. No curl finding remains
blocking under that review. The unreviewed report remains available alongside it;
other dependencies are not excluded.
The ARM64 SBOM independently passes its native hash checks and the reviewed scan
reports 7 critical and 143 high matches, with the same 45 distinct advisories and
no blocking curl finding.

Both development histories are now consolidated on `main`. The old published
reviewer ignores the optional backport policy. During bootstrap, the workflow
therefore builds the full native matrix and downloads the source reviewer's
archive and receipt from the same run. Every scan verifies its checksum,
architecture and revision, qualifies its runtime and invokes it as a Docker
action through the private candidate registry. Release still requires the entire
test matrix, including the reviewer's own negative tests and vulnerability scan.

After publishing this reviewer and pinning its verified multi-platform digest,
remove the source-reviewer preparation steps, restore the published action call
and restore `steps.sources.outputs.full` for dynamic matrix selection. This
bootstrap has not yet been exercised on GitHub. Completing it does not resolve
the remaining KAS dependency vulnerabilities.

## Hosted native evidence from run 41892997468

The run built commit `08522d32554d34f88a40351f8c110ff9a1d0a5f6` on native
AMD64 and ARM64 runners. Both KAS jobs reached the security scan; neither image
was qualified for publication. The ARM64 archive checksum is
`3600a57ff20ecd207a56af851867b3ca050dddeeea5b3fec30ef2e1945d95a33`.
The downloaded archive matches its build receipt, and 34 reviewed file hashes
were independently recomputed from its layer and compared to the hosted SBOM.
The policy now includes the ARM64 variants of the already documented backports.
These hashes do not imply a full ARM64 cold Yocto qualification.

Wget's exact `1.25.0-2+pantavisor1` native payloads were also checked against
both downloaded archives and receipts. Its finite review covers only the three
advisories addressed by the checksum-pinned filename patch and the repository's
Metalink/HTML allocation bounds patch. The source builder executes the package
tests and the ASan/UBSan regression probe before producing its packages.

Local diagnostics for this follow-up are under `/tmp/opencode/ci-fixes/`.
The ten corrected scratch images pass AMD64 native runtime qualification,
including negative fixtures and source permissions. The BSP and platform images
pass AMD64 and ARM64 scans at `high` with an empty ignore list after restoring
Debian ownership metadata. ARM64 images were built locally under emulation;
native hosted requalification remains required.

## util-linux stable security backport

The source builder uses util-linux 2.41.6 and Trixie's 2.41.5 packaging, both
checksum-pinned. The upstream release fixes failed-helper post-hooks and
restricted mount source resolution. The local patch completes the upstream
nsenter descriptor-lifetime correction, includes the resolution declarations
in the idmap hook and corrects the fallback `RESOLVE_NO_SYMLINKS` value to
Linux UAPI `0x04`. The original review of CVE-2026-78409 relied on the absence of
the 2.42 detached fast path. Later source inspection found that this argument
alone does not settle the older `open_tree` path; that advisory was removed from
the inactive backport review and remains blocking for the distribution image.

Two Debian packaging patches are already incorporated upstream. Before removing
their exact series entries, the builder requires a zero-fuzz reverse dry-run
proving each patch is present. Other patches and build errors remain fatal.

The AMD64 Debian test run completes without failures. The two extra prerequisite
probes report the kernel-denied inotify `/proc` watch and personality changes as
skipped only for `EPERM`, `EACCES` or `ENOSYS`; unexpected errors fail the test.
Existing upstream skips remain visible in the log. A separate executable links
the actual built libcommon archive and checks successful regular-file opening
plus rejection of both final and intermediate symlinks without mounting anything.

The integrated AMD64 KAS image passes runtime qualification, Python extension
checks and the HTTPS/Git/KAS fixture. Its finite advisory review requires all
six exact package versions and seven native file hashes. ARM64 util-linux
backport evidence and the final glibc/GCC integration remain outstanding.

The reviewed integrated AMD64 scan now has **0 critical and 116 high matches**,
covering five distinct advisories: GCC `CVE-2026-102010` and `CVE-2026-95619`,
and glibc `CVE-2026-19499`, `CVE-2026-5435` and `CVE-2026-84243`.
The security gate still failed for that historical image. The experimental
`gcc-build` and `glibc-build` stages were subsequently abandoned in favor of the
distribution-maintained strategy above.
