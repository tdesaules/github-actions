# Rust/GTK action primitives and runtime fixture

Only the fixed lint/build/test executables expose operations. This package handles
validated inputs, native identity, receipt hashing, deterministic resource packaging
and safe regular-file extraction. It does not accept arbitrary caller commands.

## Catalog runtime wiring contract

Image IDs: `lint-rust-gtk`, `workflow-build-rust-gtk`, `workflow-test-rust-gtk`.
Both `--tool-version` (shared workflow contract) and `--version` print `rust-gtk 1`.

Copy `testdata/application/` into a new writable temporary checkout and create an
initial Git commit. Its Rust fixture links native GTK directly (no crates), maps
a real window, supports config checking, and reports the compiled release version.
Keep all fixture directories when copying, including docs (check image-context
ignore rules if embedding it in another action image).

1. Invoke lint with `--directory=<fixture> --output=<report.json>`.
2. Invoke build with `--directory=<fixture> --version=0.1.0
   --source-sha=<fixture HEAD> --arch=<native> --output=<candidate>` and nonempty
   `GITHUB_RUN_ID`, `GITHUB_RUN_ATTEMPT` (use the actual hosted values on GitHub).
3. Invoke test with the same identity, `--archive=<candidate>/niri-tonic-files-0.1.0-linux-<arch>.tar.gz`,
   `--receipt=<archive>.receipt.json`, `--output=<qualified>`.
4. Assert qualified archive hash equals candidate hash, receipt tested:true,
   report passed:true, and output files are readable by the runner UID.
5. A tampered archive or mismatched receipt must fail before invoking tests.

For tag/published re-verification, invoke test with `--source=published`, the
already-qualified archive/receipt, and `--source-run-id` / `--source-run-attempt`
from the release marker reader. Both original run fields are required and compared;
the receipt must have tested:true. Candidate mode instead requires tested:false
and an exact current GITHUB_RUN_ID/GITHUB_RUN_ATTEMPT match. Published output
preserves preparation provenance; current verification identity lives only in
test-report.json. Receipt names remain `<archive>.receipt.json`. The additional
`extracted-root` output points to a fresh payload-only extraction for scanning.

Each action owns a Containerfile/action.yml and bootstrap metadata uses
`image: Containerfile`. The existing catalog discovery/source-action helpers and
test-candidate-runtime need explicit support for these new IDs; they are outside
this package's ownership. Publication must replace bootstrap references with real
qualified GHCR index digests. Native AMD64 and ARM64 hosted `uses:` execution is
required in addition to local Podman evidence.
