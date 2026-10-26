# Test index result

Checks that retry output is identical, both input archives retain their fixture
checksums, the index references their exact platform manifests, and the TLS
registry serves that exact index without release-version tags.

Inputs: `directory` (default `.build/fixtures`), `index`, `retry-index`.
Uses the private fixture service and its CA, without redirects or TLS bypass.
Outputs `verified`. CLI flags override `INPUT_*` values.

The fixed Go scratch image was published in `1.2.0` and is called by `test`.
