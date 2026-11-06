# Fedora KAS qualification — 2026-11-06

## Selected image

KAS remains the orchestrator; only the host build environment changes. The final
stage retains Fedora 45 and its package manager/RPM inventory instead of copying
Debian packages into scratch. Both builder stages and runtime use the allowlisted
`quay.io/fedora/fedora` repository and a verified multi-platform index:

`sha256:5ad386251aa5d0111754dff91a930acc1a928afe7bafc46762024e096ab9bbb0`

Key installed packages: curl/libcurl `8.21.0-9.fc45`, GCC `16.2.1-3.fc45`, glibc
`2.44-2.fc45`, Git `2.56.0-1.fc45`, Python 3.13 `3.13.18-1.fc45`, KAS `5.5` and
pip `26.2` (requirement `26.2.0`). The action and Git LFS use Fedora's packaged
Go `1.27.3-1.fc45`. Git LFS 3.8.1 includes klauspost/compress `1.20.2`, addressing
GO-2026-6865, alongside the existing crypto/network dependency corrections.
System compilers/libraries are not rebuilt.

Fedora 45 is outside the pinned Yocto distribution validation list. Its GCC/glibc
versions differ materially from Debian; real cold builds must establish
compatibility. A successful scan or version command is insufficient.

## Initial image: local executed checks

| Check | AMD64 | ARM64 |
| --- | --- | --- |
| Image construction | Passed | Passed under emulation |
| Archive tool-version check | Passed | Passed under emulation |
| HTTPS redirect and certificate/hostname rejection | Passed | Passed under emulation |
| Git HTTPS clone, CA rejection and KAS checkout | Passed | Passed under emulation |
| Offline Git LFS clean/pointer/smudge round trip | Passed | Passed under emulation |
| Syft RPM inventory | 485 packages | 487 packages |
| Raw Grype matches, all severities | 0 | 0 |
| Ignored matches | 0 | 0 |

The scans use an explicitly empty ignore configuration, not the historical Debian
reviews. Grype's recorded database schema is `v6.1.10`, built
`2026-11-06T06:21:59Z`, with `valid: true`. These are observed scanner results for
exact archives, not a guarantee that every possible vulnerability is absent.
Distribution feeds have different coverage; a lower counter alone did not decide
the base choice.

## Independent curl correction check

The first Fedora 44 experiment reported only one high and four medium matches.
Nevertheless, its curl changelog did not establish the known HTTP/2 upload fix
for CVE-2026-92392. That experiment and its unfinished cold build were superseded.

Fedora 45's `curl-8.21.0-9.fc45.src.rpm` was retrieved from the official source
repository location. `rpmkeys --checksig` returned **digests signatures OK**.
Its spec declares `Patch0011: 0011-multi-adjust-the-cleanup-order-in-multi_done.patch`
and applies patches using `%autosetup ... -p1`. Inspection confirms the request /
connection cleanup-order correction, not just a higher version number. The RPM
changelog explicitly names CVE-2026-92392 in the November 2 revision.

The Fedora 44 trial's other findings were corrected by upgrading the Git LFS
compression dependency and pip. No review rule or severity change was used to
clear those findings.

## Initial image: exact archive identities

| Architecture | OCI manifest digest |
| --- | --- |
| AMD64 | `sha256:d83c594775680f05566b4ca66a1a4a5e566b9d39f4fb37c0e3021fa899108550` |
| ARM64 | `sha256:917ef18df6c1904f3bd9e3f58864a250d2b117728a22c93c103e759bc9a1dd3d` |

| Architecture | Archive file SHA-256 |
| --- | --- |
| AMD64 | `7b8d380b57566cf24eac146abcadf779786d46375d92997b47c980229e42588e` |
| ARM64 | `2d9af145573b6df1d9ee76c900a363164930e22ac1739ac5909c8cb36cdadd0c` |

These are local candidate identities, not published GHCR references.

## Cold builds and external qualification

Cold build attempts for Core, k0s and System ran sequentially using the fixed
AMD64 image ID `e8f397a73ff58ca9ee620aa9a109921107b88824b89898f9b9508e24de866f21`,
which matches the scanned archive's configuration identity. All three failed:

| Project | Duration (seconds) | Failure |
| --- | --- | --- |
| Core | 2494.275 | libpcre2 RPM payload creation: cpio write failed, resource temporarily unavailable |
| k0s | 0.006 | Wrapper read a nonexistent root `name`; BitBake never started |
| System | 2003.342 | libxml2 RPM payload creation: same cpio resource error |

Cold builds establish compilation/output staging, not QEMU/platform qualification.
The zero-match scan does not establish that this initial builder is functional.

### Wrapper corrections under qualification

The k0s metadata stores its identity in `profiles.base.name`; Core uses root
`name`. The wrapper now validates the correct schema and still rejects missing
or mismatched identities.

The failed RPM command used `w3T32.zstdio`, independently of the two-worker
BitBake/make limits. Replaying that exact task alone succeeded but used up to
135 cgroup tasks. Under a controlled 128-task limit, 32 zstd workers reproduced
the same EAGAIN/cpio failure and incremented `pids.events:max` by five. Two
workers succeeded with a peak of 13 tasks and no additional limit hits. The
original containers' limit-hit counters were not retained, so this establishes
the failure mechanism under constrained resources, not the exact original
ancestor-cgroup limit.

The wrapper now appends a temporary KAS configuration setting `ZSTD_THREADS`
and `XZ_THREADS` to two, without editing producer source files or raising limits.
An initial attempt put that overlay in `/tmp`; KAS rejected concatenating files
from different repositories before BitBake started. The overlay now lives under
Git-ignored `artifacts/ci`, checked with `git check-ignore` before use, and is
removed after the invocation. Reports record `compressionThreads: 2`.
The archive identities above
remain those of the initial image; corrected-image builds, diagnostic resumptions
and fresh cold qualification must be recorded separately before promotion.

Native KAS configuration dumps accepted the repository-local overlay for all
three checkouts and retained their original hosted configuration. The actual
Core retry's generated `local.conf` also contains both two-worker limits.
The sequential validation batch completed with the repository-local overlay:

| Project | Cold | Duration (seconds) | Result |
| --- | --- | --- | --- |
| Core | No | 1606.328 | Build and output staging passed |
| System | No | 1423.363 | Build and output staging passed |
| k0s | Yes | 1842.707 | opkg-utils RPM payload creation failed with EAGAIN |

The k0s failure command explicitly used `w3T2.zstdio`; it attempted 1958 tasks
with none reused. Compression bounds reduce resource usage but do not fully
resolve the late-build failure. Core/System successes are diagnostic resumptions,
not cold qualification.

The corrected image passed local runtime/negative/permission fixtures,
HTTPS/Git/KAS/LFS checks and the installed k0s metadata helper on AMD64 and
emulated ARM64. Both raw scans again report **zero matches and zero exclusions**.

| Architecture | Corrected OCI manifest digest |
| --- | --- |
| AMD64 | `sha256:f57f50dcb87046853b499536c4de6287208e3a956b4a2336e99de267dcf52fe5` |
| ARM64 | `sha256:7a730d3e9fc44712621b49fa440db14ed9e9464c6870ac469f866f44cbc83c65` |

| Architecture | Corrected archive SHA-256 |
| --- | --- |
| AMD64 | `35b6857881ff3389841ccd1fdee7c2498f2859e2fe5aece0d9fd6f0ec26c4876` |
| ARM64 | `ee99d8647a0d845346c1a1e7c15b75d472133a32144e3f4f2f57392a1ad768b5` |

The retry builder's fixed image ID is
`47996eed7c675d40f142bb8458589dd3cdd5cdc210bf3f52741a310d64e1580f`, matching
the corrected AMD64 archive configuration. These scans still do not replace
successful consumer builds or hosted qualification.

### PID-1 lifecycle defect

The Go entrypoint waited for its direct Python child but did not reap adopted,
terminated descendants. A synthetic producer fixture exercised the actual image
entrypoint under a 128-task limit, without invoking KAS or building firmware:

| Same bounded image | Spawned orphan children | Zombies | Sampled peak tasks | Exit |
| --- | --- | --- | --- | --- |
| Default entrypoint as PID 1 | 119 of 160 | 118 | 127 | 1; cgroup limit hit |
| Podman `--init` control | 160 of 160 | 0 | 13 | 0 |

Twenty isolated replays of the failed opkg-utils RPM script, including ten under
pseudo, also passed without PID-limit hits. These results establish the wrapper's
reaping defect and distinguish it from a permanently broken package task. The
original full-build containers' zombie counts were not retained; instrumented
cold builds remain necessary to confirm the complete causal chain.

The init candidate installs Fedora's `catatonit-0.2.1-6.fc45`. When PID 1, the Go
entrypoint reexecutes that fixed init with `/entrypoint` as its child; the image's
public entrypoint remains unchanged. The child performs the existing UID/GID drop.
This avoids a custom Go wait loop racing with `os/exec`, and needs no engine
`--init` flag or increased PID limit. The durable local fixture is
`workflow/build-pantavisor/test-init.py`; it passed with the image alone on AMD64
and emulated ARM64, including propagation of an invalid-project failure. The
fixture tracks every orphan PID until reaped and leaves no surviving children.

| Init candidate check | AMD64 | ARM64 (emulated) |
| --- | --- | --- |
| Orphans created/reaped | 160 / 160 | 160 / 160 |
| Sampled peak tasks under the 128-task cap | 9 | 12 |
| Remaining children / zombies | 0 / 0 | 0 / 0 |
| Invalid-project exit propagated | 1 | 1 |
| HTTPS/Git/KAS/LFS and archive tool-version checks | Passed | Passed |
| Raw Grype matches / exclusions | 0 / 0 | 0 / 0 |

k0s then completed an instrumented diagnostic resumption in **575.647 seconds**
(`cold: false`). Catatonit was observed as PID 1; sampled peak task count was **126**,
peak transient zombie count **1**, final zombie count **0**, and recorded PID /
memory limit-hit and OOM counters remained zero. This is not a cold result.

| Architecture | Init candidate manifest digest |
| --- | --- |
| AMD64 | `sha256:aca6d457eaf57b4e673aa36d6fa99ecb29bd4fbc0e241ddde4238854e377155a` |
| ARM64 | `sha256:fa5bf5f343869bd0f2aa5cc4962af721a8fd34dcdc2153b0a2eac70f980ef57c` |

| Architecture | Init candidate archive SHA-256 |
| --- | --- |
| AMD64 | `f8e7f4247b61a6235aff08bb160f06cd95d7235b57f0b898653578c20adf0960` |
| ARM64 | `7b7c9ae430b37667573f2ed9bf1a9a0715112a44aee65cca7b79cd917699b40a` |

Fresh Core, k0s and System cold builds are now running sequentially from new
checkouts of the same producer commits, with resource sampling. Their fixed
AMD64 image ID is `b3118610aae23f9c0ef3821e421b2bd7f7a801bafcf0641b693ef9a91e535e9a`,
matching the scanned init archive. Full cold and platform qualification remain
pending.

Publication and consumer activation remain outstanding. Security reporting is now advisory under the
[new policy](ci-policy-transition.md); functional failures still block release.

## Hosted incremental qualification

[Run 42230534480](https://github.com/tdesaules/github-actions/actions/runs/42230534480)
completed successfully on `f06aa2599674f25970c9b8db1a5900f593e0e62f`:
six successful jobs (check, lint, native AMD64/ARM64 KAS builds and tests), with
release and maintenance intentionally skipped.

Both architecture reports were downloaded and inspected, independently of the
green job conclusions. SBOM generation, reviewer preparation/evaluation and
Grype execution all succeeded; each raw scan contains **zero matches and zero
ignored matches**. The advisory security policy did not conceal scanner failures.

This was an incremental KAS run, not a full-catalogue qualification. Checks for
unselected action implementations were skipped. The native runtime and invalid
KAS-project checks ran; the separate synthetic orphan-lifecycle fixture and full
consumer builds were not executed by this hosted workflow. Their local evidence
and pending cold results remain separate.

The preceding full run `42207227780` was cancelled after 241 successful jobs,
50 cancelled jobs and one skipped job, with no recorded failed job. It must not
be treated as completed catalogue qualification.

## Evidence locations

- `/tmp/opencode/kas-fedora45/{amd64,arm64}/`: exact archives, runtime reports,
  Syft inventories, Grype/database/status JSON and HTTPS/Git/LFS logs.
- `/tmp/opencode/fedora45-curl-source/`: signed source RPM, spec and patches.
- `.build/kas-fedora45-cold/`: initial/bounded builder identities and producer
  checkouts. Each project's `initial-build-report.json` preserves the first
  failed run; `bounded-build.log` and `source/artifacts/ci/build-report.json`
  hold the diagnostic retry for Core/System.
  Core's `source/artifacts/rpm-thread-reproduction.json` and `rpm-limit128-*.log`
  retain the controlled RPM resource experiment.
- `.build/kas-fedora45-k0s-cold/`: separate fresh k0s retry with the corrected wrapper.
- `/tmp/opencode/kas-fedora45-limits2/`: corrected-image builds and qualification
  artifacts, separate from the initial image and rejected outside-repository overlay.
- `/tmp/opencode/kas-init-probe/`: synthetic PID-1 comparison, not firmware output.
- `.build/kas-fedora45-k0s-cold/source/artifacts/rpm-replay-results.json`:
  isolated RPM replays with and without pseudo.
- `/tmp/opencode/kas-fedora45-init/{amd64,arm64}/`: init fixture, runtime, TLS,
  exact archives, SBOMs and raw scans for the init candidate.
- `.build/kas-fedora45-k0s-cold/init-resume-resources.jsonl` and `init-resume.log`:
  successful instrumented k0s diagnostic; `pre-init-cold-build-report.json`
  retains its failed bounded-only cold attempt.
- `.build/kas-fedora45-init-cold/`: fresh cold checkouts, fixed builder/source
  identities, resource samples and final reports for the init candidate.
- `/tmp/opencode/ci-42230534480/`: hosted native KAS diagnostics, security outcome
  summaries, SBOMs and Grype reports for both architectures.
- `/tmp/opencode/kas-fedora/`: superseded Fedora 44 experiment, retained for
  comparison rather than promoted as final qualification.
