# Package Pantavisor BSP

Invokes System's fixed `package-bsp` operation with staged deploy inputs and a
new output directory. Provides Python, Git, GNU tar, ar, zstd and depmod. System
owns the pinned vendor packages, selection and build provenance.

No build tree is rebuilt and no board is accessed. Paths are project-relative.
The bootstrap reference must become a verified digest before consumption.
