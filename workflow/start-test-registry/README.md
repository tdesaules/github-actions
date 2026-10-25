# Start test registry

A fixed-operation service image for the private GitHub job network. Starts
Distribution 3.1.2 with an ephemeral self-signed CA and a TLS listener on 5443.
Port 5000 exposes the public fixture CA and a proxy used only by the private
`candidate-registry` transport. Registry redirects remain relative so Docker can
pull candidates through the runner's mapped loopback port without daemon changes.

Use the same pinned image in separate `candidate-registry` and `test-registry`
job services. Publication/signature tests use TLS directly and verify the CA.
Keys and storage exist only in the service's temporary filesystem. The image
contains a rebuilt registry binary, not a compiler or a container engine.

There are no operation-selector inputs. The service runs until stopped by its
job lifecycle; `--tool-version` is the standard smoke-test diagnostic.
The scratch image is awaiting native qualification and immutable publication.
