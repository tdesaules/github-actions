# Incremental CI

Branch pushes compare the event's `before` and current commit using the published
`get-changed-files` and `get-image-matrix` Docker actions. Deleted paths and both
sides of renames participate in selection. New branches and unavailable previous
commits retain the detector's conservative full-selection fallback.

| Change | Selected work |
| --- | --- |
| Documentation only | Markdown and link checks; no image builds or runtime tests |
| Image recipe or copied build input | Affected images and native AMD64/ARM64 tests |
| Shared source | Images whose Containerfiles consume that source |
| Action metadata | Metadata checks and tests against the pinned image |
| Workflow or test fixtures | Applicable checks and native tests; no blanket rebuild |
| Vulnerability policy | Published-image scans, with the source reviewer dependency below |
| Empty diff | Check only |

An action still referencing `:unpublished` must be built whenever it is selected:
there is no pinned image to reuse. At the time of this change, 24 Pantavisor actions
remain in this state. Workflow-wide or policy-wide selection therefore still
builds those 48 native variants until they are qualified, published and pinned.

Lint is skipped when none of its checks have inputs. Build is skipped when no
selected image needs construction or scanning. Test accepts that skipped build
only when the check job explicitly declared it empty. Release uses the same
selected/successful versus empty/skipped gates, so skipped required tests cannot
authorize publication. Release planning keeps its previous-release comparison;
tags never build images and maintenance remains independent.

The build and test jobs retain the existing common selection matrix. In a mixed
selection, build matrix entries for published-only images perform no image build.
SBOM generation, review and vulnerability scans are advisory; functional tests
and archive integrity checks remain required.

## Temporary source reviewer dependency

The pinned reviewer predates the current backport/applicability implementation.
Until its replacement has been qualified, published and pinned, runs needing a
scan build the source reviewer once per architecture. If the reviewer is in the
selection, its normal build owns the artifact. Otherwise the first selected image
per architecture builds it as a helper and uploads a separate archive and receipt.
All test jobs wait for build completion and verify that exact same-run helper.
This requires at most two additional image builds, rather than rebuilding the
entire catalogue. Docs-only, empty and metadata-only changes need no helper.

Remove the helper build/transfer steps and source-reviewer preparation together
when switching review back to the qualified published action. Local selector and
workflow tests do not substitute for an observed hosted run on both architectures.

`scripts/ci.py` is a separate local utility with broader shared-file selection;
it does not drive the hosted workflow. Containerfile-aware selection is implemented
in `workflow/get-image-matrix` and is already available in its pinned image.
