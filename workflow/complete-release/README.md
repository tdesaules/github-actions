# Complete release

Requires a complete verified publication journal and matching release plan.
Rejects an advanced main branch, an existing tag and a dirty checkout. Updates
action digests, retains a reviewable patch, generates the changelog, commits and
atomically pushes main and the new release tag. Never force-pushes or moves tags.

Inputs: `directory`, `token` (release App token, environment-only). Credentials
are scoped to child Git processes. git-cliff receives a private temporary Git
configuration trusting only the current checkout.

The fixed Go/Git/git-cliff scratch image was published in `1.1.0` and is called
directly by the release job.
Real-Git regression tests cover success, concurrent advancement and rejected
atomic pushes. These local tests do not claim a hosted interruption campaign.
