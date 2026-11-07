# Changelog

## 1.6.0

- use approved bases for application CI tools (f30fb41)

- qualify and release native Rust GTK binaries (2afa5d0)

- validate k0s profile identity during activation (46ce2bb)

- replace source bootstraps with published control actions (b8213fe)

- reconcile published controls with binary actions (92f17f8)

- bootstrap compatible catalog qualification fixtures (45f31ef)

- rebuild selected unpublished runtime changes (d40f2a4)

- adopt runner ownership for binary action outputs (f610c10)

- map native GTK in hosted binary fixtures (6bed3f9)

- require canonical breaking-change footers (a1124b5)

- retain the upstream license filename (b8a9211)

- version the binary preparation fixture plan (ccab611)

- make native tool recipes pass shell lint (a4a4360)

- retain attempt-bound binary qualification artifacts (e44af78)

- install Git for the upstream build script (3d80984)

- include writable temporary directory in runtime (e4f07bf)

## 1.5.0

- record 1.4.3 release and force-push recovery activation (d17231f)

- add containerized build and release lifecycle (b21487f)

- explain qualification and staged consumer activation (6f93a41)

- isolate release fixtures from action directory input (ca0d9fe)

- pin the multi-platform KAS image index (6a35e37)

- refresh build dependencies and rebuild Git LFS (7bd902d)

- record final cold build and architecture scan evidence (a9e8805)

- retain Debian file ownership for vulnerability matching (c8e4709)

- backport curl 8.23 with both TLS variants (fb90f8d)

- verify native Debian backports by payload hashes (379e9a2)

- bind curl advisory review to verified native backports (853e07b)

- verify architecture-independent companion packages (060b6ae)

- rebuild vulnerable runtime dependencies from pinned sources (09c2adf)

- record verified KAS dependency backports (6b8c18d)

- replace unavailable runtime image digest (9592032)

- verify native Debian backports by payload hashes (7e3c56d)

- verify architecture-independent companion packages (531b75c)

- replace unavailable runtime image digest (79fcd76)

- consolidate Pantavisor CI and backport review on main (08522d3)

- provide sticky temporary directories in scratch actions (ea7bc18)

- retain Debian ownership in minimal tool images (315ac73)

- bootstrap scans with the verified source reviewer (8e9c101)

- backport util-linux security corrections (e5be911)

- verify native backport and util-linux evidence (d925fdf)

- use the upstream-maintained Debian runtime (d092b60)

- record cold Core qualification on the upstream runtime (40a95c3)

- recognize eight verified curl backport fixes (c5ec8fc)

- verify bounded applicability against OCI inventories (131934e)

- record seven verified KAS applicability proofs (10e146f)

- prefer the stable curl security update (4f54284)

- record remaining security blockers and Core qualification (3ce8c0d)

- restore incremental image selection (7a91847)

- separate advisory security from functional qualification (889a22c)

- retain maintained Fedora packages in the build runtime (175b486)

- read k0s identity from its base profile (7c9f4c5)

- bound compression workers with a repository-local overlay (9161e64)

- reap orphaned build processes with packaged init (f06aa25)

- record successful hosted incremental qualification (e90d8b2)

- record successful cold builds for all consumers (625f5c0)

- add publication-free full catalogue qualification (129dbc4)

- record both QEMU platform qualifications (def168e)

## 1.4.3

- close audit findings with verified hosted checksum updates (b148271)

- update module github.com/go-jose/go-jose/v4 to v4.1.7 (f21dab2)

- update module github.com/moby/go-archive to v0.3.3 (b959344)

- update module github.com/rhysd/actionlint to v1.7.12 (e28d244)

- update module github.com/sigstore/cosign/v3 to v3.1.4 (76daf04)

- update node.js to v24.22.2 (093b2ff)

- update quay.io/buildah/stable docker tag to v1.43.5 (e5ad9c9)

- recover missing base commits after force pushes (6e0a99b)

## 1.4.2

- isolate checksum downloads from runner HOME (1a38996)

- expose hook results and branch status failures (8c382dd)

- record OIDC proof and remaining checksum runtime validation (55d5d38)

## 1.4.1

- record full 1.4.0 qualification and maintenance results (867ea9d)

- commit post-upgrade artifacts through Git (7b47473)

- include patched npm for lockfile updates (be164cc)

- record hosted Renovate artifact findings and fixes (6a39c45)

## 1.4.0

- update docker.io/library/golang docker tag to v1.27.3 (da3c02d)

- update node.js to 8a32c8c (026f67b)

- record catalogue review and verified qualification evidence (ab1ecd3)

- keep OIDC identity tokens out of process arguments (50362a2)

- update Go tool source pins with verified checksums (eb5b664)

- record fixes and remaining hosted validation for audit findings (48ed06e)

- read identity fixtures without external cat (3c892b9)

## 1.3.1

- activate published scan-only and retry-aware action interfaces (6f0bd9c)

- clarify fail-closed evidence for published image scans (903e293)

- bind reuse selection to the actual checkout (ac8070b)

## 1.3.0

- activate Docker-composed build and test jobs (a628cfe)

- keep private signing config out of diagnostics (beddcec)

- forward registry credentials through the environment (f2c4fe5)

- derive rebuilds from build context dependencies (2ff203a)

- retain native published archives for scan-only jobs (107db8d)

- select complete immutable results across retries (890c0a4)

- enable dedicated maintenance and complete manager coverage (3ed7b6d)

- record prerequisite publication and activation checks (109174c)

- update unavailable Alpine package pin (30e7560)

- replace vulnerable Node runtime (c350a30)

- rebuild inherited tools with patched go-jose (5ec1901)

- track tool recipes and exclude release-owned images (e4c9e67)

- record publication scan failures and remediation evidence (243c22c)

## 1.2.0

- activate native release and maintenance actions (8a40c3b)

- separate image builds from native qualification (18cd3c5)

- update vulnerable Zod and Undici dependencies (4717cc8)

- download every page of qualification artifacts (9d73698)

## 1.1.0

- activate published Docker orchestration actions (6af5a11)

- add native release and maintenance stages (1dbab55)

- remove shell dependency from registry fixtures (e379bd6)

## 1.0.1

- retire bootstrap checks and add native orchestration (2e94dd6)

- scope libgit2 trust to the mounted checkout (d551bc6)

- replace vulnerable markdown-it with upstream fix (1eb0171)

## 1.0.0

- compose selective checks from published native actions (a7169c2)

- record published orchestration and check images (1064f08)

- record successful hosted selective activation (c54839b)

- expose dedicated linters directly in the workflow (7bf9146)

- add selective containerized catalogue and test suites (71deccb)

- replace unavailable Skopeo bootstrap digest (dc751b8)

- allow execution of temporary Go test binaries (f807268)

- avoid failing HTTP/2 module downloads (316031e)

- bootstrap Skopeo from the published catalogue (74122e1)

- preserve sticky temporary directory permissions (35b759d)

## 0.3.0

- remove host Python from the Buildah launcher (0b29f62)

- add Markdown, link and action metadata checks (5792510)

- add selective planning and qualification actions (345d3c0)

- add a containerized Renovate action (8799e5d)

- qualify the new check and orchestration candidates natively (6a1a669)

- distinguish orchestration building blocks from activation (b632581)

- exclude runner fixtures from catalogue validation (66e8918)

- transfer candidates through an isolated registry (86f20c0)

- record the published 0.2.0 qualification evidence (f51b13b)

- include Git in the Go test environment (d3703d5)

- rebuild selected images that are not published yet (9a539e2)

- remove the vulnerable Markdown CLI glob dependency (007cb07)

- remediate Renovate cache and brace vulnerabilities (a944b39)

- preserve Markdown spacing between changelog sections (63ca3e9)

## 0.2.0

- rename the catalogue workflow to main (e79a29b)

- record the first stable automated release (083ab0c)

- exercise publication recovery and atomic push races (07b369a)

- add five independently containerized repository checks (0bffb12)

## 0.1.0

- add native Docker actions and bootstrap pipeline (3209df8)

- qualify hosted Docker namespace profiles (6a0f32d)

- qualify scratch builder with hosted runtime profile (3caa51f)

- record hosted Buildah qualification and runtime decision (6436ddf)

- use qualified Docker profile for Buildah operations (75f71ee)

- rebuild vulnerable tools and qualify public Buildah launchers (f11249c)

- prepare signed semantic releases with atomic refs (be63ff8)

- preserve index children and exercise native registry actions (7a1aae6)

- resume publication from source-matched tested artifacts (108cada)

- check HTTP links and configure dependency updates (5cb81d5)

- separate workflow and publication concurrency locks (644bb15)

- document the published bootstrap and verified consumer path (f8ab432)

- share one Grype database across architecture rescans (b3a4ec7)

- record successful hosted maintenance and final qualification (4ecf0c6)

- specialize action images and publish indexes by digest (da5083d)

- explain per-action image migration and qualification status (ddc6bd7)

- pin ten signed action-specific image indexes (dc12fb5)

- retire superseded shared packages after replacement verification (d9e32d2)

- record specialized image publication and GHCR signature behavior (c254b42)

- record successful migration and retirement of shared images (b723aa2)
