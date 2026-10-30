# Changelog

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
