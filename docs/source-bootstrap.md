# Temporary Rust/GTK catalog bootstrap

An unpublished action may name only its own `Containerfile`. Discovery derives
`family-directory` and verifies the regular recipe's final-stage OCI title.
Such entries have an empty registry reference and always rebuild when selected.
They cannot be reused as published images or validated on release tags.

Catalog validation requires explicit `allow-source: 'true'` (local CLI:
`python3 scripts/ci.py validate --allow-source`). Action metadata qualification
uses `ALLOW_SOURCE_ACTIONS=true`; release tags reject both exceptions. Normal
validation remains digest-only. No placeholder registry digest or tag is created.

The source bridges in `main.yml` bootstrap the changed selector, catalog and
metadata validators, artifact-reuse validator, fixture generator, runtime tester,
publication planner and publisher using `prepare-source-action`. The existing
source finalizer now replaces owned Containerfile metadata with the observed
published digest. This prevents the old selector/publication controls from
deadlocking first publication of new actions.

## Qualification

Native AMD64 and ARM64 jobs import each exact candidate OCI archive into the
isolated registry and execute literal candidate `uses:` steps. Rust fixtures use
a dependency-free synthetic application with real Cargo/Node/Python checks,
archive checksums, source-bound receipts and headless Weston. Application CI
remains responsible for testing the complete real application.

The eighth action, `workflow/summarize-security`, qualifies real severity counts,
advisory tool errors and unsafe-output rejection. Its fixture and runtime controls
must be included in the same qualification/publication and pin-update cycle.

Release runtime fixtures exercise the actual fixed-operation executables with
ephemeral TLS trust, a non-forwarding loopback HTTPS proxy, an isolated local Git
remote and dummy credentials. They verify exact asset bytes, idempotent uploads,
bound artifact downloads, corruption rejection, and metadata/tag preparation.
No cloud authentication or remote repository writes are used by these fixtures.

## Removal condition

After hosted qualification, publish the **unchanged tested archives** for both
architectures and resolve their real registry/index digests. Pin all new actions
and updated controls, including the finalizer, before enabling consumers. Then:

1. Replace `.source-actions` invocations with their pinned ordinary action paths
   and remove their corresponding `prepare-source-action` bridge steps.
2. Remove `SOURCE_CONTROLS` and the duplicate published/source fixture/runtime
   paths, keeping the now-published fixture generator and runtime tester.
3. Remove `allow-source`/`ALLOW_SOURCE_ACTIONS` from workflow validation steps.
4. Run strict catalog validation and another native hosted qualification using
   the published controls; confirm the actual hosted run before consumer use.

Existing unrelated reviewer/security/finalization bridges have their own prior
publication dependencies. This change does not silently claim they are retired.
