# Build native Atomic OCI archive

**BOOTSTRAP ONLY:** `runs.image: Containerfile` is temporary source-build metadata.
Activate consumers only after native AMD64 and ARM64 hosted qualification and
publication, replacing it with the exact verified action image digest.

## Scoped Docker-socket exception

This is the user-approved **targeted BuildKit** exception for Atomic OS image
construction. The 269-layer, approximately 8.7 GB uncompressed OS exhausted
hosted runner storage under VFS deep-copy amplification. This dedicated native
Docker action uses the runner's `/var/run/docker.sock` to create one temporary
Buildx `docker-container` builder with overlay-backed BuildKit storage.

GitHub's Docker-action launcher supplies that socket. The action fixes its Docker
endpoint to `unix:///var/run/docker.sock`; it cannot select a remote engine.
Buildx manages its own builder container and state volume. Its docker-container
driver uses a privileged **builder container**; the action itself does not request
privileged execution. No socket is passed into the image build, no additional
host paths are mounted, and no insecure BuildKit entitlements are enabled here.
This exception does not alter `oci/build-image` or its qualified VFS/chroot
launcher, and does not authorize sockets in other catalog actions.

## API

| Input | Default | Meaning |
| --- | --- | --- |
| `context` | `.` | Workspace-relative build context |
| `containerfile` | `Containerfile` | Workspace-relative regular Containerfile, not relative to context |
| `image` | required | Untagged image name, e.g. `ghcr.io/niri-tonic/niri-tomic-os` |
| `version` | required | Candidate image tag or no-v release version |
| `platform` | required | Exactly `linux/amd64` or `linux/arm64` |
| `target` | empty | Optional build stage |
| `labels` | `{}` | Trusted JSON object of string OCI labels |
| `build-args` | `{}` | JSON object of non-secret string build arguments |
| `output` | `image.oci.tar` | New workspace-relative OCI archive file |

Outputs:

- `archive`: **workspace-relative** path, usable by subsequent host/action steps.
- `archive-sha256`: SHA-256 of the exact archive bytes, not its manifest.
- `manifest-digest`: validated `sha256:...` digest of the single native OCI image
  manifest referenced by `index.json`, not the config digest. Emitted from the
  same archive verification pass, without importing or reconstructing the image.
- `size`: archive byte count.

Context, Containerfile, image/version, labels, build arguments and target retain
the corresponding `oci/build-image` input meanings. Unlike its historical
absolute archive output, this action returns a portable workspace-relative path.
There are no secrets, registry credentials, arbitrary builder image, command,
operation, extra mount, cache destination, push or load inputs.
Use trusted source/Containerfiles and non-secret inputs. It is a fixed build
operation, not a general runner-engine interface.

Both the Go action architecture and the Docker server's Linux architecture must
match the requested platform. Multi-platform builds and emulation are rejected;
use separate native runner jobs and merge the **qualified** archives later.

Paths cannot be absolute, contain parent traversal, exporter CSV separators or
control characters, or resolve outside the workspace. Existing output files,
directories and dangling links are rejected. The Containerfile and context are
not rewritten; arrange `.dockerignore` in the source to exclude prior build
artifacts from the uploaded context. The private client config is outside it.

## Fixed operation and artifact integrity

1. Validate all inputs before contacting the engine.
2. Query the Docker server platform using API negotiation with the pinned CLI.
3. Create a cryptographically unique private Buildx builder, using the pinned
   BuildKit index below. It never selects a global/default builder.
4. Bootstrap it, then build with a single native `--platform`,
   `--provenance=false`, `--sbom=false` and
   `--output type=oci,dest=<private staging path>,oci-mediatypes=true`.
   No image is pushed or loaded into the runner's image store.
5. Verify the OCI layout, exactly one native OCI image manifest, configuration
   platform, requested labels, every blob hash and descriptor size. Hash the
   exact archive in the same sequential pass without unpacking layers or making
   another full archive copy. Metadata reads are bounded to 8 MiB each.
6. Remove only this builder and its state. Exclusively hard-link the unchanged
   verified archive (mode `0644` for host artifact transfer) to the requested
   output, then remove private staging/config.
   A concurrent output writer is never overwritten.

Construction is **not functional qualification**. Feed the archive/checksum to
`check/test-atomic-image`, then use the qualified artifact for ISO construction
and publication. No `tested: true` or boot-test claim is generated here.
Constructor checks can use `manifest-digest` for embedded-OS identity; consumers
still obtain independent manifest verification from `check/test-atomic-image`.
SBOM, Grype and signing remain separate advisory operations.

## Lifecycle and failure behavior

Server discovery is limited to 30 seconds, builder creation to one minute,
bootstrap to five minutes and construction to two hours. SIGINT/SIGTERM cancel
the client and its Buildx child process group. Cleanup gets a fresh, independent
one-minute deadline even after cancellation; client pipe shutdown is bounded.

Bootstrap, build, verification or cleanup failures block output publication.
Cleanup failures identify the uniquely named builder requiring attention. There
is no global prune and no deletion of other builders, images or volumes. Builder
creation is separate from bootstrap, so a failed create cannot authorize removal
of an existing unrelated builder. Failed builds remove their private partial
archive and client configuration.

Do not concurrently mutate the trusted checkout while it is being built.
Uncatchable SIGKILL/runner termination cannot guarantee cleanup; the disposable
hosted runner must discard any remaining resources.

## Pinned toolchain and provenance

- Runtime: Fedora 45 index
  `sha256:b174cd5906c712f59a675cc85ea2ca8ff3bcf3643f1a1f3bc4e14dc28bcf1872`.
- Go builder RPM: `golang-1.27.4-2.fc45`; statically compiled action.
- Docker CLI 29.9.3 / Buildx v0.39.0 donor:
  `docker.io/library/docker:29.9.3-cli@sha256:5a7af0dcb0e46c803efe9b45371dd21cb500e5b4453250394ac6f06d49b6e6f1`.
  This upstream Alpine 3.24 donor is build-only; the final runtime is Fedora.
  CLI commit `5457199aa57c45e990c59c89cab5f02b817238d7`;
  Buildx commit `360040a78d6113dc84e5d4dcdf0f438e1be7a41f`.
  Both copied binaries are static; their versions and runtime execution are
  checked locally. Upstream LICENSE/NOTICE bytes are fetched from those source
  commits, SHA-256 verified, and retained under `/usr/share/licenses`.
- BuildKit v0.35.0:
  `docker.io/moby/buildkit@sha256:aa632b921ef07411d2cb77292cd34213d4f7b384001d6fceef1e1f25bf9752bb`.
  Observed native child manifests:
  AMD64 `sha256:a521fc1fd903a46c611c733dd692548512232fba2e0386d6055846a903d05a71`;
  ARM64 `sha256:7dc61a9fab7c7f5a50b1cdbbefa4180829e68c6c0172206eb54ad934de097ddf`.
  The index is pinned internally, not caller-configurable.

## Validation and hosted activation

Run from the catalog root:

```text
go test ./workflow/build-atomic-image
go vet ./workflow/build-atomic-image
podman build -f workflow/build-atomic-image/Containerfile \
  -t localhost/workflow-build-atomic-image:qualification .
podman run --rm localhost/workflow-build-atomic-image:qualification --tool-version
```

Fixtures mock Docker operations without a socket and exercise native platform
guards, literal label arguments, escaping paths, output collisions, failed
bootstrap/build/cleanup, cancellation, malformed archives and exact bytes.

Local rootless Podman checks validate the produced runtime and bundled CLI/plugin,
not a Docker daemon build. **Docker CLI 29.9.3 against hosted Docker 28.0.4 remains
a hosted compatibility gate**, including native ARM64. The positive hosted tests
must build and qualify the actual native Atomic artifacts, record disk use and
verify owned builder/state removal before digest-pinned consumer activation.
Do not start a privileged local Docker daemon to substitute for that evidence.
