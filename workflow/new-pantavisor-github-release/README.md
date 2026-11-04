# New Pantavisor GitHub Release

After immutable publication and signature verification, rechecks qualification
and the tag-bound receipt digest, then creates the GitHub Release containing the
source/version/component/image manifest. An identical existing release is reused;
a conflicting release is refused. Also writes a public release-manifest JSON.

Requires contents write permission. Registry publication and signatures remain
explicit separate steps. GitHub-token credentials are environment-only.
