# Repository guidance

## Verification

- Run commands from the repository root. Local tooling: Python 3.11+, Go 1.25.5+ (`go.mod`), and Task.
- `task test-repository` runs image-contract validation, Python unit tests, then `go test ./...`; it does not run container integration tests or the CI linters.
- Focus Python tests with `python3 -m unittest discover -s tests/unit -p 'test_ci.py' -v`; focus Go tests with `go test ./shared/runtime -run TestName` (substitute the package/test).
- `task test-action-metadata` is a separate containerized metadata check and installs pinned PyYAML inside the validator container.

## Wiring and contracts

- CI orchestration lives in Go actions under `workflow/`, invoked by `.github/workflows/main.yml`. Local Task commands use `scripts/*.py`; these are separate implementations. For selection/publication changes, inspect both paths rather than assuming the Python scripts drive CI.
- Actions are discovered from `action.yml` and adjacent `Containerfile` files, not a central catalog. Each image has one owner. Image IDs normally use `<family>-<action>`; moved `lint/` actions retain their published IDs. Workflow composites without a Containerfile are excluded from image discovery.
- Discovery parses image references with indentation-sensitive regexes: preserve two leading spaces for `image: docker://...` and eight for `BUILDER_IMAGE: docker://...`.
- OCI `main.go` files delegate to `shared/runtime`; changing shared behavior can affect multiple action images. Other action families also use shared Go packages in the single root module.
- CI image selection is implemented in `workflow/get-image-matrix/`, including Containerfile dependency analysis. The main workflow forwards the changed-file selector's `full` output; bootstrap control tools and the reviewer dependency are separate from selected product builds.
- Catalog validation requires named stages, a last stage named `final`, and digest-pinned external images. Prefer scratch for self-contained tools; build environments may retain a compatible allowlisted distribution. Apply the base allowlist to builders and donors too; report existing migration debt explicitly. Build contexts are the repository root.

## Container development

- The local engine defaults to Podman; select Docker with the environment variable `CONTAINER_ENGINE=docker`.
- `task initialize-bootstrap` builds and smoke-tests all images on the host architecture, building `oci-build-image` first. A focused build (`task build-image IMAGE=oci-export-image`) requires the local builder image at the same `VERSION` first; the default version is `0.0.0-bootstrap`.
- After bootstrap, `task test-integration` exercises real containers. `task test-image IMAGE=oci-export-image` only checks tool version, architecture, and entrypoint.
- Build artifacts live in `.build/<image>/<arch>/`. `task new-candidate-actions` generates `.candidate-actions/` for local-image action tests: native `docker://` references would otherwise pull published images even when a candidate is loaded locally.
- Rootless Podman tests and `scripts/images.py --direct` builds do not qualify GitHub's Docker execution. Hosted qualification requires real `uses:` steps on both native architectures; see `docs/runners.md`.
- Container outputs must use shared workspace paths. Absolute paths returned inside an action are container paths; downstream host steps should use the configured output path.

## Publication

- Publication requires tested AMD64 and ARM64 OCI archives with verified checksums. Keep publication archives unchanged; Docker-load conversion is only an ephemeral execution transport.
- Published actions reference GHCR images by digest. SemVer versions are Git tags without a `v` prefix and GitHub Releases, not OCI SemVer tags. Unchanged images retain their digests.
- Security reports (SBOMs, vulnerability reviews/scans, signing and verification) are advisory, including tool errors. Preserve real outcomes separately from functional qualification. Build/test failures, archive/source/platform mismatches and publication failures remain blocking.
