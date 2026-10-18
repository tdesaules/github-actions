# New candidate action

Candidate action; its image has not yet been published.

Provide `directory` (source action), `image` (already loaded `localhost/...:tag`)
and `destination` (separate generated directory). The operation preserves
inputs and arguments, replaces Docker image metadata with a local Dockerfile,
and supports the narrow Buildah composite image override. An immutable registry
reference (`registry/repository@sha256:...`) instead generates a direct
`docker://` action reference, allowing native tests without loading a host image.

It does not pull or load images. Explicit CLI flags override `INPUT_DIRECTORY`,
`INPUT_IMAGE` and `INPUT_DESTINATION`. Source metadata remains untouched.
