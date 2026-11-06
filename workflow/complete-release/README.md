# Complete release

Requires a complete verified publication journal and matching release plan.
Signature success is advisory and is not required; published-artifact integrity
and complete functional qualification remain required.
Rejects an advanced main branch, an existing tag and a dirty checkout. Updates
action digests, retains a reviewable patch, generates the changelog, commits and
atomically pushes main and the new release tag. Never force-pushes or moves tags.

Inputs: `directory`, `token` (release App token, environment-only). Credentials
are scoped to child Git processes. git-cliff receives a private temporary Git
configuration trusting only the current checkout.

The new runtime retains Fedora's packaged Git environment. Git-cliff 2.14.2
comes from upstream release assets with architecture-specific SHA-256 checks and
retained licenses. The release job bootstraps this source implementation until
its new immutable image is published; the older published image required signatures.
Real-Git regression tests cover success, concurrent advancement and rejected
atomic pushes. These local tests do not claim a hosted interruption campaign.
