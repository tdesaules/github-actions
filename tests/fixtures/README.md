# Consumer action fixtures

`component/` is a synthetic schema-2 integrity fixture. Its rootfs and SBOM files
are literal test bytes, not executable images or usable archives. It must never
be published as a Pantavisor release. Its descriptor records `dirty: true`.

`pantavisor-system/` implements only the `check` command used to exercise the
System action's actual Python, Git, shell, GNU cpio and GRUB dependencies. It is
not a copy of the System project or a substitute for its regression suite.
