# Read Pantavisor published release

Reads the latest stable GitHub Release's immutable publication manifest for
maintenance. Validates the version/tag and digest-qualified references. A missing
first release emits `has-release: false`; malformed or inaccessible releases fail.
No build or publication is performed.
