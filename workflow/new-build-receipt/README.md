# New build receipt

Records `build.json` with `tested: false` only after the archive matches the
builder's SHA-256 output. Binds the archive to its image, architecture, source
revision and candidate version. Does not run tests or produce release metadata.

Inputs: `archive`, `output`, `expected-sha256`, `image`, `architecture`,
`revision`, `version`. CLI flags override `INPUT_*` values. A receipt cannot
overwrite its archive. The fixed Go scratch image was published in `1.2.0` and
is called directly by the build job.
