# Rust/GTK lint

Fixed source checks for niri-tonic-files: `cargo fmt --check`,
`cargo clippy --all-targets --locked -j2 -- -D warnings`, Node syntax under
plugins, Python compilation in memory, desktop-file validation, and direct
package/lock dependency coherence. Does not run unit tests or release builds.

Inputs: `directory` (`.`), `output` (`.build/lint/rust-gtk.json`). Paths are relative
to the original workspace. The JSON report records success or the failing check.
CLI flags override INPUT_* values. No command/script input exists.

Fedora 44 supplies packaged Rust/Clippy/rustfmt, GTK development libraries,
Node/npm, Python and desktop-file-utils; this cannot use a scratch runtime.
The Go executable is static and built with the catalog's Fedora 45 toolchain.
Docker starts as root then adopts a non-root checkout owner's UID/GID, preserving
runner output ownership. Root-owned local checkouts retain their owner.

Bootstrap metadata deliberately uses `image: Containerfile`. Publish and qualify
native AMD64/ARM64 images, then replace it with the actual GHCR index digest.
Run `go test ./lint/rust-gtk ./shared/rustgtk` from the catalog root.
