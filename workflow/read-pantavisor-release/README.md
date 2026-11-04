# Read Pantavisor release

Requires an annotated Pantavisor release tag whose commit belongs to main. Checks
the preparation run's exact base revision, workflow path, event and successful
conclusion for the exact recorded attempt through GitHub's API. Briefly waits
for that attempt to finish after its atomic push. Verifies the retained artifact
belongs to that run and returns its exact ID, run ID and expected receipt digest.
Missing/expired artifacts must fail, not trigger an implicit rebuild.
