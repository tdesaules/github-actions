# Build Pantavisor

Builds `kas/ci.yml` for `core`, `k0s` or `system` inside the action, without invoking
a container engine or requiring privileged mode, socket access or host sysctls.

## Runtime choice

The candidate uses the official Fedora 45 multi-platform base pinned at
`sha256:5ad386251aa5d0111754dff91a930acc1a928afe7bafc46762024e096ab9bbb0`.
Fedora supplies the system compiler and libraries as binary RPM packages.
The full distribution runtime is intentional: a Yocto builder needs Python,
shells, Git, compilers and development utilities. Keeping RPM ownership and
dependency management is simpler than manually copying a closure into scratch.
Fedora minimal is an alternative to measure, not an assumed security improvement.

KAS 5.5 and its Python dependencies are pinned in `requirements.txt`, installed
as binary wheels in `/opt/kas`. Fedora's Python 3.13 package preserves the
interpreter series of the previous working BitBake baseline instead of using
the distribution's newer default interpreter. Fedora 45 is outside the pinned Yocto host validation
list: version smoke tests alone do not establish build compatibility.

Git LFS 3.8.1 retains its checked module checksum, explicit x/crypto, x/net and
klauspost/compress 1.20.2 fixes, and license. The venv uses pip 26.2.0. The action
and Git LFS are built with Fedora's packaged
Go toolchain. No system compiler/library is rebuilt. Historical Debian collectors
and source-backport scripts remain for evidence but are not invoked.

The image digest pins the parent, not the mutable Fedora update repositories.
RPM updates are resolved at construction; SBOMs record exact installed versions.
Publication must use the exact tested archives, never a reconstruction.

## Qualification

`scripts/test-pantavisor-curl.py --curl-version 8.21.0 --no-gnutls --no-http3`
tests HTTPS, certificate/hostname rejection, Git and KAS checkout against an
isolated server (provide `--reference` and `--platform`). Fedora Git uses the
OpenSSL libcurl backend, unlike the former Debian GnuTLS variant. The TLS trust
bundle is `/etc/pki/ca-trust/extracted/pem/tls-ca-bundle.pem`.

The pinned curl `8.21.0-9.fc45` changelog explicitly records CVE-2026-92392's
cleanup-order correction. The first Fedora 44 experiment had few scanner matches
but did not establish that fix, so it was superseded rather than declared safe.

Debian-specific reviews do not apply to Fedora packages. Preserve raw scanner
findings and actual tool errors; the catalogue's security policy is advisory.
Local native AMD64 and emulated ARM64 checks, cold consumer builds, hosted native
qualification, publication and consumer activation are separate evidence stages.
See [Fedora qualification](../../docs/pantavisor-fedora-qualification.md) for exact
local scan/runtime evidence and pending cold/hosted checks,
[architecture](../../docs/architecture.md) for runtime and security policy,
and [historical remediation evidence](../../docs/pantavisor-kas-remediation.md)
for the superseded Debian baseline.

## Build contract

The wrapper starts as root to create `/work` and writable output directories,
then drops to the checkout owner's UID/GID and clears supplemental groups.
This is not complete sandbox isolation: GitHub can mount its engine socket and
inject service tokens; effective access still needs hosted measurement.

Existing build/staged-output directories are refused by default. `cold: 'false'`
permits diagnostic recovery without overwriting an export and is recorded in
the report. Consumer workflows keep the cold default. Provenance binds source
and deploy outputs. Files are staged under `artifacts/ci/deploy` before removing
this invocation's build directory; `artifacts/ci/build-report.json` records
duration, status and disk space. An unpublished candidate is not a consumer pin.

Core's identity comes from `build/component.json` root `name`; k0s uses
`profiles.base.name`. Both are checked against the selected project.
A temporary KAS overlay bounds zstd and xz compression to two workers, matching
the hosted compilation profile. `PARALLEL_MAKE` alone does not limit RPM payload
compression. KAS requires concatenated files in the same repository, so this
overlay lives in generated `artifacts/ci` files and is verified Git-ignored
before use. Producer source files stay unchanged; the report records
`compressionThreads: 2`. Diagnostic resumptions are never labelled cold builds.
