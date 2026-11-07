# Rust/GTK catalog bootstrap retirement

The catalog's 1.6.0 preparation run `42384772496` qualified native AMD64 and
ARM64 images and published their tested archives. Action metadata now contains
the observed registry digests. The workflow activates those published controls
through ordinary action paths, including the selector, validators, fixture
generator, runtime tester, Rust fixture builder, publisher and release finalizer.

The temporary preparation helper and duplicate source-control execution paths
have been removed. Workflow validation is strict, without source exceptions.
Candidate `uses:` qualification and source fixtures remain for all eight new
actions. A subsequent native hosted run must confirm this activated workflow;
local validation alone does not establish hosted execution.

## Historical bootstrap contract

An unpublished action may name only its own `Containerfile`. Discovery derives
`family-directory` and verifies the regular recipe's final-stage OCI title.
Such entries have an empty registry reference and always rebuild when selected.
They cannot be reused as published images or validated on release tags.

Catalog validation requires explicit `allow-source: 'true'` (local CLI:
`python3 scripts/ci.py validate --allow-source`). Action metadata qualification
uses `ALLOW_SOURCE_ACTIONS=true`; release tags reject both exceptions. Normal
validation remains digest-only. No placeholder registry digest or tag is created.

The former source bridges in `main.yml` bootstrapped the changed selector, catalog and
metadata validators, artifact-reuse validator, fixture generator, runtime tester,
publication planner and publisher using the retired preparation helper. The
source finalizer replaced owned Containerfile metadata with the observed
published digest, avoiding a first-publication deadlock in the older controls.

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

## Activation verification

Run strict catalog validation and native hosted qualification using the
published controls. Confirm both architectures execute the candidate fixtures,
including source permissions, negative inputs and exact-archive integrity checks.
The six jobs retain separate construction, qualification and publication phases.

The unrelated reviewer/security bridges retain their existing qualification
dependencies.
