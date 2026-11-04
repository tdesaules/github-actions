# Package Pantavisor component

Invokes only `build/package.py --deploy-dir ... --output ...` in a producer
checkout. The producer retains ownership of its schema and provenance checks.
The deploy directory defaults to `artifacts/ci/deploy`; output defaults to
`dist/component` and must be new. Paths are project-relative and passed literally.

The dedicated scratch image contains Python, Git and their runtime dependencies.
No build or publication occurs. Publish the bootstrap image before consumption.
