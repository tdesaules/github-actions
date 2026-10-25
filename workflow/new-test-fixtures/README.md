# New test fixtures

Fixtures now include untested build receipts, archive checksums, suite cases with
spaces, and native successful/exit-42 RUN executables. These executables match
the fixture image's architecture, so Buildah tests need no external base pull.
Files adopt the runner UID when GitHub output ownership is available.

The image is published and pinned by digest in tag `0.3.0`.

Provide `directory` to generate positive/negative check fixtures and
deterministic `fixture.oci.tar` (AMD64) and `arm64.oci.tar` archives. The archives
contain a small file and a writable sticky `/tmp` directory.

`--directory` overrides `INPUT_DIRECTORY`. Use a generated-artifact directory;
fixture files with matching names are replaced.
