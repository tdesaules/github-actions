# Test catalog publication

Reject tags that differ from the checkout, are outside main's history, or refer
to unpublished/mismatched OCI indexes or incomplete platform sets. This check
remains blocking. Cosign verification is a separate advisory operation.

The package-managed Fedora runtime provides Git and Skopeo; all build/runtime
bases are allowlisted and digest-pinned. Until publication, the release workflow
uses the checksum/platform-verified source bootstrap, never an invented digest.
