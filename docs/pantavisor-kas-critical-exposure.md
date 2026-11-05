# KAS critical findings: exposure assessment

## Scope and decisions

This assessment starts from the seven advisory IDs responsible for the 22
critical package/advisory matches after applicability review. Hosted run
[42098220154](https://github.com/tdesaules/github-actions/actions/runs/42098220154)
confirmed the seven existing applicability proofs on both architectures; only
the two KAS scans failed, with 62 remaining high/critical IDs.

Sources inspected include the exact Debian Git 2.47.3 source/patch set, curl
8.21.0 source and upstream advisories, installed KAS 5.5 Python modules, the
consumer workflow template, and the pinned Poky checkout used by the successful
Core build (`b2c16f1e69130558574e1363aff944647fa9cd11`). Consumer workflows are
still pending activation: template intentions are not evidence of a completed
hosted producer build. No exploit was executed and no new exclusion was added.

The user selected the **stable Debian security stream for curl** after the
branch comparison below. System libraries are not rebuilt locally.

## Important additional finding: curl branch security divergence

[CVE-2026-92392](https://curl.se/docs/CVE-2026-92392.html) is an HTTP/2 upload
use-after-free. Curl's upstream advisory lists **8.21.0 as affected**, but the
hosted Grype reports contain no match for this ID on either architecture.

The exact stable package `8.14.1-2+deb13u7` explicitly fixes this advisory:

- Its Debian changelog names the fix in the `trixie-security` upload.
- `debian/patches/CVE-2026-92392.patch` changes the cleanup order in
  `lib/multi.c`, releasing request/filter references before rescheduling handles.
- The `8.21.0-2~bpo13+1` packaging patch series does not contain that correction.

The Trixie feed's fixed-version ordering is insufficient to assess a different
upstream backports branch. A higher upstream number is not proof of a superset
of stable security fixes. Git's smart HTTP protocol sends POST request bodies,
so HTTP/2 upload handling deserves priority even though this particular failure
was not reproduced against Git.

The image now selects `curl`, `libcurl3t64-gnutls` and `libcurl4t64` at
`8.14.1-2+deb13u7`, without the backports repository or its HTTP/3 dependency
overrides. This is a deliberate tradeoff: the eight earlier reviews bound to
8.21.0 do not apply to the stable packages. Counters may increase while this
specific, previously unreported issue is corrected. The `high` gate remains
blocking and no risk is implicitly accepted.

Both local image variants pass native-tool/negative-fixture qualification and
HTTPS/Git/KAS integration checks (ARM64 under emulation). Their curl executable
and TLS libraries match the authenticated stable Debian packages. The inventory
diff is limited to 50 curl/dependency/package-cache paths per architecture;
all seven existing applicability package/version and native-file witnesses are
unchanged. After revalidating their complete inventories, the reviewed scans
report **30 critical / 215 high on AMD64** and **30 critical / 180 high on ARM64**,
with **70 distinct blocking IDs**. The stable-security cold Core build subsequently
passed in **2325.496 seconds**. See the [remaining findings report](pantavisor-kas-remaining.md)
for the complete advisory inventory and refreshed official-package availability.

## The original seven critical advisory IDs

| Advisory | Required condition | Evidence for current callers | Decision |
| --- | --- | --- | --- |
| CVE-2026-18924 | HTTP/2 Server Push plus shared connections | Curl's `lib/http2.c` enables push and enters the cloning path only with `multi->push_cb`. Neither Git 2.47.3 nor curl's CLI source configures `CURLMOPT_PUSHFUNCTION` | Stock callers lack that prerequisite; the library still exposes it to other programs, so no image-wide exclusion |
| CVE-2026-19931 | Negotiate authentication, empty credentials and connection reuse across identities | Git `http.c` supports `CURLAUTH_ANY`, GSS/Negotiate and empty authentication; curl CLI supports `--negotiate`. Public KAS repository URLs do not configure this authentication, but credentials/configuration can be supplied later | Conditional and supported; not proven absent for the generic build action |
| CVE-2026-86414 | HTTP CONNECT proxy, origin credentials, malicious 401 response from the proxy | KAS preserves proxy environment settings. Git reads configured proxy settings plus HTTPS/ALL_PROXY variants; credential helpers remain configurable | Proxy/credential conditions must be measured and controlled; no exclusion |
| CVE-2026-92388 | FTP wildcard LIST, then handle reuse/cloning or callback mutation | Neither stock Git nor curl CLI sets `CURLOPT_WILDCARDMATCH`; CLI URL globbing is not that API. BitBake's normal HTTP/FTP fetcher invokes wget | Prerequisite not observed in stock callers, but callable libcurl API remains in the image |
| CVE-2026-19445 | Python TLS server whose SNI callback swaps context and releases the original context | No SNI callback/context replacement found in KAS, the inspected BitBake libraries or project code. Current fetch paths are TLS clients. Recipes and their tests can execute arbitrary Python | Low exposure in observed fetch paths, not proof of absence from all builds |
| CVE-2026-40468 | Crafted gawk operation; CNA specifies at least 6 GB available memory | Unlike CVE-2026-40469, this is not restricted to 32 bit. Neither the action nor consumer template guarantees an effective process memory bound below that threshold | Keep blocking; no inference from a runner label or typical memory use |
| CVE-2026-6653 | Malicious XML/internal subset/entity handling | Installed `2.12.7+dfsg+really2.9.14` is actually based on affected 2.9.14. Debian explicitly still lists it vulnerable. Native libxml2 and its Perl consumers remain present | Keep blocking; the earlier Python-binding-only review covers a different CVE |

Curl rates the four original curl issues Low/Medium. CERT-PL rates
CVE-2026-40468 Low (CVSS 4.0 2.1) with its memory prerequisite. Debian describes
several findings as minor/no-dsa. These distinctions inform prioritization but
do not automatically replace Grype's severity or clear the gate.

The stable choice also re-exposes the eight version-bound findings previously
fixed in 8.21.0. Of those, four are classified critical by this scanner:
CVE-2026-10536 (stream-dependency/reset API sequence), CVE-2026-11856 (Digest
authentication state), CVE-2026-8926 (netrc/URL-user password handling), and
CVE-2026-9079 (proxy password handling). The stock Git and stable curl CLI
sources do not set `CURLOPT_STREAM_DEPENDS`; the authentication cases remain
conditional, not excluded. The stable CLI does call `curl_easy_pause`, so no
general claim that it avoids every relevant callback API is made.

## Data and credential boundaries

- `templates/pantavisor-producer.yml` separates build/test from release, defaults
  to `contents: read`, and uses `persist-credentials: false` for checkout.
  Publisher/App secrets are not explicitly passed to the KAS build step.
- This does **not** make the Docker action credential-free. Logs from hosted job
  `127336571106` contain Docker invocations with `ACTIONS_RUNTIME_TOKEN` and the
  runner-mounted `/var/run/docker.sock`. No OIDC request-token variable appeared
  in those invocations. Values were not collected or printed.
- GitHub's container-action handler adds those runtime resources independently
  of the action's need for them. The action does not invoke a container engine,
  but must not claim that the runner guarantees socket absence.
- The wrapper drops UID and clears supplemental groups. Its primary GID is
  derived from the checkout. Actual socket access under that UID/GID has not
  been measured in a hosted producer build; non-root alone is not proof that
  access is denied.
- KAS filters the environment for child commands, but its Python parent starts
  with the inherited environment. Child filtering alone is not a complete
  credential-isolation argument for processes running under the same UID.
- Build output integrity is an asset even without publishing credentials: a
  compromised builder can alter artifacts that later stages consume. Functional
  smoke tests and source revision receipts do not prove absence of a backdoor.

## Fetch and parser boundaries

The three KAS projects pin their layer revisions and use HTTPS repository URLs.
The wrapper provides a temporary Git configuration containing scoped checkout
trust. KAS can still add a configured `GIT_CREDENTIAL_HELPER`; its environment
setup explicitly preserves lowercase HTTP/HTTPS/FTP proxy settings and
`SSH_AUTH_SOCK`.

The inspected BitBake wget fetcher supports HTTP, HTTPS, FTP and FTPS, checks
certificates by default and verifies a download checksum after the transfer.
These checks are useful, but the network client necessarily parses responses
before a completed-download checksum can be verified. A pinned commit or
archive checksum is not a mitigation for a vulnerable network parser.

The action executes `kas/ci.yml` from the supplied checkout and arbitrary recipe
commands. Absence of an option in the three current project files cannot be
promoted into an intrinsic image-wide non-applicability proof. This assessment
does not claim that every recipe input or mirror is authenticated and pinned.

## Prioritized follow-up

1. Complete cold Core and hosted qualification of the selected stable curl
   security packages. The local proofs have been revalidated without changing
   their semantic conditions, positive witnesses or expiry. Keep historical
   8.21.0 reviews version-bound.
2. Before publication, measure the effective build UID/GID and Docker socket
   permissions on hosted producers. Prevent engine access without changing host
   daemon permissions or adding capabilities. Removing inherited Actions service
   tokens before Python/KAS startup is another bounded mitigation to validate.
3. For the supported public-fetch profile, explicitly assess proxy, credential
   helper and ambient authentication settings. Forcing HTTP/1.1 for managed Git
   is an optional mitigation to evaluate, not a substitute for a corrected
   libcurl package or an image-wide proof about arbitrary clients.
4. Retain the TLS-client-only distinction for Python; if a recipe starts a TLS
   server, verify SNI context lifetime or require a corrected interpreter.
5. Assess resource bounds for gawk and XML consumer/input requirements. Validate
   compatibility before removing dependencies or imposing build-wide limits.
6. Any residual applicable issue needs an official fix or an explicit, scoped,
   expiring risk decision. None of those risk acceptances has been made here.

## References and local evidence

- [curl upstream advisory records](https://curl.se/docs/vuln.json)
- [Python SNI advisory](https://www.cve.org/CVERecord?id=CVE-2026-19445)
- [CERT-PL gawk record](https://cveawg.mitre.org/api/cve/CVE-2026-40468)
- [Debian libxml2 status](https://security-tracker.debian.org/tracker/CVE-2026-6653)
- [GitHub container-action handler](https://github.com/actions/runner/blob/main/src/Runner.Worker/Handlers/ContainerActionHandler.cs)

Evidence directory: `/tmp/opencode/kas-critical-exposure-42046121374/`.
It retains verified Debian Git sources, the stable curl source descriptor and
packaging archive, authenticated native binary packages, and qualification/scan
reports. The cold-build Poky sources remain under
`.build/kas-upstream-core-cold/source/artifacts/layers/poky/`.
