# Publish catalog images

Publishes the exact archives authorized by `new-publication-plan`, then verifies
the remote index and both platform digests. Rechecks archive hashes immediately
before use. Skopeo preserves digests; TLS verification stays enabled. No build
occurs, no release-version image tags are written, and retries retain the same
content-addressed destinations. Reused images are verified without rebuilding.

Inputs: `directory`, `registry-username`, `registry-password`. Credentials are
environment-only and stored in a private temporary auth file, removed on exit.
Updates `publication.json` after each verified image. Signing is a separate action.

The fixed Go/Skopeo scratch image was published in `1.1.0` and is called directly
by the release job.
