# New catalog SBOMs

Discovers pinned catalogue images from source metadata and scans both AMD64 and
ARM64 variants using Syft. Writes SPDX and Syft JSON with Go symbol and file-hash
evidence. No image is rebuilt. An unpublished image fails closed.

Input `output` defaults to `.build/maintenance`. Reports are stored under
`<image>/<architecture>/`. Individual scan failures are collected while other
images continue. Vulnerability evaluation is a separate action.

The fixed Go/Syft scratch image is being qualified before activation.
