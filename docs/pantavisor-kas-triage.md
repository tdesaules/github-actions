# KAS advisory triage — 2026-11-05

Scope: the exact AMD64 and ARM64 SBOMs from
[run 42018140474](https://github.com/tdesaules/github-actions/actions/runs/42018140474),
commit `40a95c3811f3fc16551237ed37c6b1f9d63e74c1`. This is the reduced action
image with official Debian curl backports, not the unmodified Siemens image.
No system libraries were rebuilt for this triage.

## Result

Eight of the original 77 distinct blocking advisories are demonstrably fixed in
the installed curl version. The finite review now recognizes only those fixes.
The other **69 remain blocking**, including eight candidates for a more precise
applicability review; those candidates are not exclusions.

| Architecture | Original critical / high matches | Reviewed critical / high matches |
| --- | --- | --- |
| AMD64 | 39 / 294 | 23 / 278 |
| ARM64 | 39 / 236 | 23 / 220 |

Counts are package/advisory matches, not distinct CVEs. Each reviewed curl CVE
appears on four binary packages. Both reviewed scans still fail at `high`.
No risk acceptance, severity override or broad package exclusion was added.

## Verified curl remediation

Installed version: `8.21.0-2~bpo13+1` for `curl`, `libcurl4t64`,
`libcurl4-gnutls` and the transitional `libcurl3t64-gnutls` package.

For each advisory below, curl's upstream OSV record explicitly identifies
`8.21.0` as fixed and does not list it among affected releases. Debian's sid
fixed-version records agree. The Trixie records still describe the older
distribution source version with no upper affected-version bound.

| Advisory | Upstream correction |
| --- | --- |
| [CVE-2026-10536](https://curl.se/docs/CVE-2026-10536.html) | HTTP/2 stream-dependency tree use-after-free |
| [CVE-2026-11856](https://curl.se/docs/CVE-2026-11856.html) | Cross-origin Digest authentication state leak |
| [CVE-2026-12064](https://curl.se/docs/CVE-2026-12064.html) | Default-protocol SSH verification bypass |
| [CVE-2026-8926](https://curl.se/docs/CVE-2026-8926.html) | netrc password leak with a URL user |
| [CVE-2026-8932](https://curl.se/docs/CVE-2026-8932.html) | Incomplete mTLS configuration matching |
| [CVE-2026-9079](https://curl.se/docs/CVE-2026-9079.html) | Stale proxy password leak |
| [CVE-2026-9080](https://curl.se/docs/CVE-2026-9080.html) | Pause/socket callback use-after-free |
| [CVE-2026-9545](https://curl.se/docs/CVE-2026-9545.html) | HTTP/3 early-data exposure |

Verification performed:

- Downloaded both architectures' four binary packages through authenticated APT
  indexes and compared the curl executable and both TLS library SHA-256 values
  to the hosted SBOMs. All six native file hashes match.
- Verified upstream/packaging source archive hashes and inspected the Debian
  patch series. It contains no additional fix for the later 23 curl advisories.
- Required all four exact package versions and all three native files per
  architecture before emitting the eight advisory reviews.
- Altering an executable hash or changing the package version emits zero rules
  on both architectures. Valid evidence emits exactly 32 package-scoped rules.
- Re-ran Grype against both hosted SBOMs; only these eight advisory IDs cease
  blocking. The existing negative review tests also pass.

The remaining 23 curl advisories explicitly list 8.21.0 as affected. Their fixed
releases are 8.22.0 or 8.23.0 on this release line. Fixes in the separate 8.14.x,
8.16.x or 8.20.x maintenance lines do not establish that 8.21.0 is fixed.
Curl rates these remaining issues Low or Medium; that severity disagreement is
documented, but does not automatically override the scanner's High/Critical gate.

## Complete remaining source-package inventory

The counts below sum to all 69 remaining advisory IDs. Detailed package/version
and file-level evidence is retained in the original scan artifacts. Trixie
tracker entries are open for all these rows; `no-dsa` or a “Minor issue” note is
not treated as proof of non-applicability.

| Source package | Installed version baseline | Remaining IDs | Next evidence/action |
| --- | --- | ---: | --- |
| curl | 8.21.0-2~bpo13+1 | 23 | Review actual protocol/authentication/callback use; await applicable official fixes |
| libxml2 | 2.12.7+dfsg+really2.9.14-2.1+deb13u3 | 8 | Separate Python-binding-only issue from native parser issues |
| python3.13 | 3.13.5-2+deb13u5 | 5 | Trace XML, HTML, TLS callback/wrap_bio and archive extraction callers |
| expat | 2.9.0-1~deb13u1 | 4 | Native and Python XML parsing paths; upstream fixes are in 2.10.0 |
| gawk | 1:5.2.1-2+b1 | 4 | Separate 32-bit-only arithmetic issue; review native extensions and build inputs |
| util-linux | 2.41.5-0+deb13u1 | 4 | Separate 2.42-only fast path; review mount/nsenter privileges and use |
| glibc | 2.41-12+deb13u4 | 3 | strfmon formats, TSIG debug functions and LANGUAGE environment trust boundary |
| sqlite3 | 3.46.1-7+deb13u2 | 3 | Check FTS5/session configuration and origins of database/changeset inputs |
| wget | 1.25.0-2 | 3 | Metalink, filename conversion and HTML conversion paths |
| acl | 2.3.2-2+b1 | 2 | Path ownership and privileges during metadata/archive operations |
| gcc-14 | 14.2.0-19 | 2 | Separate aligned-new implementation selection from the confirmed heap template defect |
| perl | 5.40.1-6+deb13u1 | 2 | Archive::Tar and Pod::Text inputs; no assumption that build inputs are harmless |
| cyrus-sasl2 | 2.1.28+dfsg1-9 | 1 | DIGEST-MD5 plugin presence and loading paths |
| gnupg2 | 2.4.7-21+deb13u1 (native payloads +b5) | 1 | TPM daemon presence and launch paths |
| krb5 | 1.21.3-5+deb13u1 | 1 | KRB-CRED consumer and forwarded-credential use |
| ncurses | 6.5+20250216-2 | 1 | Whether the affected infocmp executable is present |
| rsync | 3.5.0+ds1-0+deb13u1 | 1 | Sender/delta-transfer use with a malicious receiver |
| zlib | 1:1.3.dfsg+really1.3.1-1+b1 | 1 | Verify whether the affected later gz_vacate implementation exists in this source |

Authenticated APT queries against Trixie, security and backports found no newer
applicable binary version for the other 17 source-package groups on AMD64.
Do not substitute sid binaries or restart compiler/library source rebuilds merely
to reduce these counts.

## Applicability candidates — still blocking

These eight IDs are promising candidates, not accepted exceptions:

| Advisory | Observed boundary | Proof still required before automated review |
| --- | --- | --- |
| CVE-2026-24882 | `tpm2daemon` is absent from both complete hosted file inventories | Bind absence and exact package payloads; reject a future image that adds the daemon |
| CVE-2025-69720 | `infocmp` is absent from both inventories; the advisory concerns its command implementation | Bind executable absence rather than ignoring all ncurses findings |
| CVE-2026-107161 | No DIGEST-MD5 plugin payload appears in either inventory | Verify module ownership/loading paths and enforce absence of all plugin aliases |
| CVE-2026-74860 | No `libxml2mod` or `libxml2.py` binding appears in either inventory | Verify that the affected implementation belongs exclusively to the missing Python bindings |
| CVE-2026-40469 | Upstream description limits this issue to 32-bit gawk; both native packages are 64-bit architectures | Verify the actual executable ABI and source condition, not just the image architecture |
| CVE-2026-95619 | Upstream describes the overflow in non-POSIX aligned allocation branches | Verify selected implementation for every shipped shared/static and biarch library |
| CVE-2026-78409 | Upstream identifies a fast path introduced in util-linux 2.42; this source is 2.41.5 | Check the exact Debian patch set for a backport of the affected path |
| CVE-2026-85091 | Upstream affected range begins at zlib 1.3.1.2; installed upstream baseline is 1.3.1 | Check the exact Debian patches and affected function, not the version string alone |

The file observations use Syft's `files.selection=all`, squashed image scope and
file-metadata cataloger on both hosted SBOMs. The current reviewer does not
enforce absent-component conditions, so none of these observations produces an
ignore rule. A forged or incomplete inventory must never establish absence.

## Next decisions

1. Finish the eight applicability proofs and add only evidence-enforced reviews
   whose prerequisites can be checked on every scan.
2. Prioritize network/download/archive parsers and any credential-bearing paths
   among the remaining version-affected findings. A successful build is not a
   security regression test.
3. Where no official correction exists, record the actual exposure and mitigation
   before proposing a finite, owned, expiring risk acceptance. No such acceptance
   has been made here; the current reviewer is not a risk-acceptance engine.
4. Follow distribution fixes and re-evaluate the pinned base; keep publication
   blocked until the chosen policy is satisfied.

## Evidence snapshots

Local evidence directory: `/tmp/opencode/kas-triage-42018140474/`.

| Input | SHA-256 |
| --- | --- |
| [Debian tracker JSON](https://security-tracker.debian.org/tracker/data/json) | `66be24c6fabaa39f82d2fcf884e5de644da91c36a4eb17cabc3b56a1d24d911b` |
| [curl upstream OSV records](https://curl.se/docs/vuln.json) | `97e81f9a06590abd5641ce14269f613eac26fc109bc7f57aa1df1390110dc164` |

The directory also contains downloaded source archives, native Debian packages,
positive/negative review outputs, and both reviewed Grype reports with database
metadata. These are local evidence files, not a publication or a hosted rerun.
