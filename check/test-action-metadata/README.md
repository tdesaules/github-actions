# Test action metadata

Action validating this catalogue's action contracts. Supply `files`
as a nonempty JSON array of metadata paths. Native actions must own their
Containerfile and Go executable and forward inputs explicitly. Composites must
assemble actions; the Buildah launcher is the sole host-command exception.

Reports per-file results on stdout and sets `checked` and `passed`. The
`--files` CLI flag overrides `INPUT_FILES`. The image is pinned by digest in tag `0.3.0`.
