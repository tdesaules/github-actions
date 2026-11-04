# Read Pantavisor components

Reads `compositions/components.json` (schema 1), requiring immutable GHCR Core and
k0s component references. Emits them without downloads or registry credentials.
The file is version-controlled in System; no floating tags or fabricated initial
references are supplied by the migration.
