# Render BlueBuild recipe

Validate a trusted BlueBuild recipe and generate a Containerfile for the requested
platform. The publication namespace is forwarded to BlueBuild so its signing
module configures the intended registry identity.

This action renders only. Use `workflow/build-atomic-image` after its qualification to build
the generated Containerfile, then qualify the exact archive before publication.
The renderer and builder must share the checkout and generated BlueBuild scripts.
Generated Containerfiles include the resolved base digest; retain them as build
evidence. Modules with floating references remain floating unless the recipe pins
them explicitly.

The runtime uses Fedora for Git and TLS support. Podman is installed because
BlueBuild checks its version even when only generating a Containerfile.
The BlueBuild executable is
copied from a pinned multi-platform upstream installer image. No container is
started and no engine socket is accessed when rendering with the OCI inspection driver.
The version-only probe uses VFS and `_CONTAINERS_USERNS_CONFIGURED=done` to avoid
Podman's namespace re-exec under Docker's default seccomp profile. This does not
grant capabilities or invoke a nested container build.

## Bootstrap status

The source `Containerfile` reference is temporary. Replace it with the digest of
the exact AMD64/ARM64-qualified image before activating a released consumer.
Unit tests validate paths, registry identity, and architecture inputs; they do
not replace a real recipe-rendering smoke test on both native runners.
