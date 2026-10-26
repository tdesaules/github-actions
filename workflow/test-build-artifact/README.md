# Test build artifact

Verifies a downloaded archive against its untested build receipt and expected
matrix context. Rejects qualified metadata used as a receipt, wrong image,
architecture, revision or version, and changed archive bytes.

Inputs: `archive`, `receipt`, `image`, `architecture`, `revision`, `version`.
Outputs `verified` and `archive-sha256`. Explicit CLI flags override `INPUT_*`.
This fixed Go scratch image does not qualify the image or rebuild anything.
Published in `1.2.0`, it checks the archive both before and after qualification.
