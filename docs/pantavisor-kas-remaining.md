# KAS remaining security findings — 2026-11-06

Historical Debian baseline, before the Fedora experiment and advisory-security
policy. The counts and former publication gate below are preserved as evidence,
not the current Fedora result. See [the policy transition](ci-policy-transition.md)
and [the current builder](../workflow/build-pantavisor/README.md).

## Result

No additional package correction is available in the selected maintained Trixie
baseline at this audit. The latest applicable correction remains commit
`4f54284`: stable curl `8.14.1-2+deb13u7` fixes CVE-2026-92392, which the
8.21.0 backport lacks. No new vulnerability has been cleared by this audit.

The reviewed local scans still contain 70 distinct blocking IDs on each
architecture: AMD64 **30 critical / 215 high** matches and ARM64
**30 critical / 180 high** matches. These are package/advisory match counts.
Every remaining ID maps to an open Trixie advisory in the Debian tracker.

The scope remains official maintained packages, without rebuilding system
libraries/compilers or mixing unstable binaries into Trixie. No additional
image-wide non-applicability claim is established here. The `high` gate and
existing seven bounded reviews are unchanged; no residual risk is accepted.

## Availability verification

APT indexes were refreshed with signature verification in the pinned Siemens
parent, with both AMD64 and ARM64 indexes. `apt-cache policy` checked all 14
affected source families through representative binary packages. Trixie,
Trixie security and Trixie backports were inspected. Backports was added only
to the disposable audit container, not to the production Containerfile.

All versions below already match the available stable/security candidates on
both architectures. Curl is the only listed package with a newer backport;
returning to it would reintroduce the known HTTP/2 security defect.
The Siemens `latest-debian-trixie` index is still
`sha256:0f929318babc0291ab940d6d6db535906dd32e4cf6a0914f0cd19b42ae7453f9`.

| Source family | Current binary version | Blocking IDs | Obstacle / next action |
| --- | --- | --- | --- |
| curl | 8.14.1-2+deb13u7 | 31 | Await a security revision or a backport also fixing CVE-2026-92392 |
| libxml2 | 2.12.7+dfsg+really2.9.14-2.1+deb13u3 | 7 | No corrected candidate; six issues lack a no-DSA annotation |
| python3.13 | 3.13.5-2+deb13u5 | 5 | No corrected candidate; TLS-client use does not prove absence from arbitrary recipe tests |
| expat | 2.9.0-1~deb13u1 | 4 | Latest security package remains affected; two issues lack a no-DSA annotation |
| util-linux | 2.41.5-0+deb13u1 | 4 | No corrected candidate; older open_tree path prevents excluding CVE-2026-78409 |
| glibc | 2.41-12+deb13u4 | 3 | CVE-2026-19499 and CVE-2026-5435 are marked for the next point update, not yet available |
| gawk | 1:5.2.1-2+b1 (AMD64), 1:5.2.1-2+b2 (ARM64) | 3 | No corrected candidate; critical memory defect is not the reviewed 32-bit-only issue |
| sqlite3 | 3.46.1-7+deb13u2 | 3 | No corrected Trixie candidate |
| wget | 1.25.0-2 | 3 | No corrected candidate; required by BitBake fetch paths |
| acl | 2.3.2-2+b1 | 2 | Debian plans unstable then a point update, explicitly not individual patch backports |
| perl | 5.40.1-6+deb13u1 | 2 | CVE-2026-9538 postponed pending upstream regression fixes |
| gcc-14 | 14.2.0-19 | 1 | No corrected candidate; heap defect differs from reviewed aligned allocation |
| krb5 | 1.21.3-5+deb13u1 | 1 | No corrected Trixie candidate |
| rsync | 3.5.0+ds1-0+deb13u1 | 1 | Latest security package remains affected; no no-DSA annotation |

Debian's `no-dsa: Minor issue` annotations are prioritization decisions, not
non-applicability proofs. Grype's `wont-fix` classification does not mean a fix
can never arrive. Neither authorizes ignoring an advisory under this policy.

## Complete remaining inventory

All identifiers below have the `CVE-2026-` prefix. Each occurs on both
architectures; library and multilib matches are grouped by source family.

| Source family | CVE-2026 suffixes |
| --- | --- |
| curl | 10536, 107928, 107929, 11856, 12064, 13608, 18924, 19931, 80229, 80230, 80255, 82209, 86409, 86410, 86414, 87862, 87863, 87864, 87865, 8926, 8932, 9079, 9080, 92386, 92387, 92388, 92389, 92390, 92391, 94266, 9545 |
| libxml2 | 6653, 86138, 86139, 86140, 86142, 86143, 86144 |
| python3.13 | 15308, 19445, 19553, 7210, 82049 |
| expat | 113351, 115506, 117610, 118370 |
| util-linux | 76642, 78408, 78409, 78410 |
| glibc | 19499, 5435, 84243 |
| gawk | 40467, 40468, 40553 |
| sqlite3 | 119373, 119374, 119379 |
| wget | 58469, 58471, 58472 |
| acl | 54369, 54370 |
| perl | 82560, 9538 |
| gcc-14 | 102010 |
| krb5 | 107778 |
| rsync | 119741 |

## Qualification and follow-up

- Existing local runtime and HTTPS/Git/KAS checks passed on AMD64 and emulated
  ARM64. This audit made no executable changes and did not repeat those tests
  or claim a new scan qualification.
- Hosted run [42128105730](https://github.com/tdesaules/github-actions/actions/runs/42128105730)
  targets `4f542840ebd150dd316f8b000f6a306e65f8982e`; it was queued when inspected.
  Hosted confirmation remains outstanding.
- The stable-security cold Core build completed successfully during this audit:
  `cold: true`, `success: true`, `durationSeconds: 2325.496` (about 38m45s).
  It used `localhost/github-actions/workflow-build-pantavisor:security-amd64`
  and a fresh checkout under `.build/kas-security-core-cold/source`.
  Its report is `source/artifacts/ci/build-report.json` under that build folder.
  This is local functional qualification, not hosted or security qualification.
- Monitor glibc's announced point update, ACL's planned update, Perl's upstream
  regressions, and the unannotated Expat/libxml2/rsync findings. Rebuild and
  revalidate both architectures and exact inventory proofs when packages land.
- Runtime token stripping and effective hosted UID/GID/socket-access checks
  remain separate exposure-reduction follow-ups. They do not fix the library
  defects or justify removing these 70 IDs from the scan.
- Publication and consumer activation remain blocked. Removing required tools,
  changing distribution, or accepting residual risk requires a separate
  compatibility/risk decision rather than counting as an available fix.

## Evidence

- [Debian tracker JSON](https://security-tracker.debian.org/tracker/data/json),
  retrieved for all 70 IDs and checked against their Trixie source records.
- Local tracker snapshot: `/tmp/opencode/kas-remaining-debian-tracker.json`.
- Authenticated APT availability: `/tmp/opencode/kas-remaining-apt-policy.txt`.
- Exact-image reviewed reports:
  `/tmp/opencode/kas-critical-exposure-42046121374/{amd64,arm64}/grype-reviewed.json`.
- [Critical exposure analysis](pantavisor-kas-critical-exposure.md) records
  caller prerequisites and the stable/backports curl tradeoff.
- [Remediation history](pantavisor-kas-remediation.md) and
  [applicability evidence](pantavisor-kas-triage.md) retain earlier decisions.
