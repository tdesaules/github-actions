# KAS advisory triage — 2026-11-05

Initial scope: the exact AMD64 and ARM64 SBOMs from
[run 42018140474](https://github.com/tdesaules/github-actions/actions/runs/42018140474),
commit `40a95c3811f3fc16551237ed37c6b1f9d63e74c1`. This is the reduced action
image with official Debian curl backports, not the unmodified Siemens image.
No system libraries were rebuilt for this triage. Applicability verification
uses the subsequent native archives and SBOMs from
[run 42046121374](https://github.com/tdesaules/github-actions/actions/runs/42046121374),
commit `c5ec8fc3d9930bab6f7284d01aaae2fd0028a9cb`.

## Result

Eight of the original 77 distinct blocking advisories are demonstrably fixed in
the installed curl version. The finite review now recognizes only those fixes.
That first review left 69 blocking IDs. Seven of the eight subsequent
applicability candidates are now verified, leaving **62 blocking IDs**.
The util-linux candidate remains unresolved and is not excluded.

| Architecture | Original critical / high | After curl fixes | After applicability proofs |
| --- | --- | --- | --- |
| AMD64 | 39 / 294 | 23 / 278 | 22 / 222 |
| ARM64 | 39 / 236 | 23 / 220 | 22 / 187 |

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

## Inventory after the initial curl review

The counts below describe the 69 IDs before applicability verification. Detailed package/version
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

## Applicability verification

All eight candidates were investigated. Seven now have bounded, automatically
revalidated proofs; util-linux remains blocking:

| Advisory | Observed boundary | Verification result |
| --- | --- | --- |
| CVE-2026-24882 | Missing TPM daemon | `tpm2d/command.c` belongs to separately packaged `tpm2daemon`; verify its absence in the actual OCI layers and bind the installed GnuPG payloads |
| CVE-2025-69720 | Missing infocmp command | `analyze_string` occurs only in `progs/infocmp.c`, not the shipped libraries; independent archive inventory excludes the executable |
| CVE-2026-107161 | Missing DIGEST-MD5 plugin | `add_to_challenge` occurs in `plugins/digestmd5.c`; installed core/DB libraries match official packages, and affected plugin paths, package and fingerprint are absent |
| CVE-2026-74860 | Missing Python libxml2 binding | Broken Python SAX `attributeDecl` reference handling is in `python/libxml.c`, not the native shared library; enforce binding/module/package absence |
| CVE-2026-40469 | 32-bit-only gawk issue | CERT-PL's CNA record restricts the platform to 32 bit; the exact authenticated executables matching both images are ELFCLASS64, EM_X86_64 / EM_AARCH64 |
| CVE-2026-95619 | Unaffected POSIX aligned-new implementation | All four native/biarch config headers select POSIX; disassembly of shared aligned-new routines and `new_opa.o` members of all eight static archives confirms `posix_memalign` |
| CVE-2026-78409 | Version argument insufficient | **Still blocking:** 2.41.5 has an older `open_tree` path in `hook_subdir.c`; absence of the 2.42 detached fast path alone is not a sufficient proof |
| CVE-2026-85091 | Installed source predates affected implementation | CNA affected range is 1.3.1.2–1.3.2; the exact 1.3.1 source has no `gz_vacate`, and its Debian patch series is empty |

### Enforcement and validation

The rules in `verified_debian_applicability` expire exclusively at
**2026-12-05 00:00 UTC** and name `tdesaules/github-actions` as their review owner.
The maximum review window is 31 days. They require the exact image-owner label,
distribution, native architecture, package/version maps and positive payload
hashes, plus source references. They are non-applicability proofs, not permission
to accept a vulnerable runtime merely because it runs in CI.

Each variant additionally binds the complete inventory fingerprint (all recorded
paths/types, payload hashes, links, modes and ownership). New files, unknown
replacement binaries or changed permissions invalidate all applicable proofs
until the tree is revalidated. Timestamps and OCI labels are excluded so a new
run ID alone does not invalidate an identical tree. The two independent hosted
runs have identical payload, inventory, mode and ownership records.

All positive native files were compared to exact Debian packages downloaded
through authenticated APT indexes. The absent components' official packages
were also downloaded for fingerprints, without installing or executing them.
Reintroduced paths, aliases, package artifacts or renamed identical native
payloads invalidate a component-absence proof. The GCC heap-template advisory
CVE-2026-102010 remains blocking.

An experiment found that Syft 1.54.1 does **not** record `--exclude` in its
descriptor: `files.selection=all` cannot independently establish completeness.
The reviewer now requires the exact OCI archive and verifies its manifest,
config and layer hashes against the SBOM identity. It streams a separate final
file inventory, applying whiteouts and hard links without extraction or execution.
Filename/fingerprint absence and positive native hashes are checked directly
against that inventory. SBOM JSON cannot inject the trusted archive inventory.

Negative tests cover a component hidden by a Syft exclusion, reintroduced or
renamed components, aliases, altered payloads, changed versions, missing/wrong
archives, expiry, duplicate evidence, unsafe paths, corrupt gzip data and tar
content hidden after an end marker. Package names are escaped for Grype's regex
matching, including literal `g++` and `libstdc++` identities.

The native AMD64 reviewer and emulated ARM64 reviewer both generate the expected
proofs against the hosted archives. Rescans remove exactly these seven additional
IDs, retain util-linux's disputed finding and all other unreviewed IDs, and still
fail at `high`. The updated reviewer/workflow must be qualified on GitHub before
these local results are called hosted qualification. Saved review configurations
are not a fresh evaluation of expiry; regenerate them on each scan.

The older, inactive util-linux source-backport policy also no longer excludes
CVE-2026-78409 on the strength of the version-only argument.

## Next decisions

1. Qualify the updated reviewer and archive forwarding on hosted runners; keep
   the unresolved util-linux finding blocking pending a stronger proof or fix.
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

Applicability evidence is under `/tmp/opencode/kas-applicability-42046121374/`:
exact hosted archives with checked receipts, source/CNA snapshots, authenticated
positive and absent-component packages, native hashes, reviewer build/execution
logs, evidence reports and final reviewed scans for both architectures.
