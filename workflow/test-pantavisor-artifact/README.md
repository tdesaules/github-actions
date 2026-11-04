# Test Pantavisor artifact

Verifies the full transferred inventory, expected source revision and optionally
the tag's receipt digest. `require-tested` additionally requires qualification;
System needs the combined base/k0s receipt. Returns the version and, when present,
the root image manifest digest from the inventoried OCI archive. Does not rebuild.
