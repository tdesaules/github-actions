# New Pantavisor candidate

On a clean main checkout, calculate the stable release version from Conventional
Commits, update VERSION/changelog and create a detached candidate commit. The
first release uses the existing VERSION. Documentation-only changes after a
release do not bump; fix/perf/refactor bump patch, feat minor, breaking changes
major. Export a Git bundle and candidate metadata under the ignored artifacts
directory; do not push or move main.

All subsequent build/test jobs must import this exact commit. A release cannot
change VERSION after its artifacts have been built. Publication is separate.
