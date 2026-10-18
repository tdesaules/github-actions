# New test fixtures

Candidate action; its image has not yet been published.

Provide `directory` to generate positive/negative check fixtures and
deterministic `fixture.oci.tar` (AMD64) and `arm64.oci.tar` archives. The archives
contain a small file and a writable sticky `/tmp` directory.

`--directory` overrides `INPUT_DIRECTORY`. Use a generated-artifact directory;
fixture files with matching names are replaced.
