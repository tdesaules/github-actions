# Get Pantavisor qualification

Lists all artifact API pages for the current preparation run and selects the
latest retained base/k0s results for the exact build artifact ID. Attempt-specific
names avoid overwriting earlier evidence. Results are transferred by artifact ID;
the merge action still checks source, version, profiles and full file inventories.

This action does not rely on colliding matrix outputs or a single API page. It
requires actions read permission and never modifies artifacts or releases.
