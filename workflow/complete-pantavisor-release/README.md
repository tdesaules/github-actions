# Complete Pantavisor release

Requires the exact candidate checkout and qualified artifacts. Records the
qualification receipt digest, exact artifact ID and successful preparation
attempt in an annotated `v<VERSION>`
tag, then atomically pushes the candidate to main and the new tag with a release
App token. Rejects advanced main, an existing tag and incomplete System profiles.

The token is confined to the Git child environment. No force push, workflow-token
tag trigger assumption, or tag movement is used. A tag event publishes the already
qualified outputs; this operation does not publish registry content.
