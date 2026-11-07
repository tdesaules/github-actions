# Runtime qualification fixture

Copy to a temporary directory and initialize a Git checkout before action execution.
This small Rust binary links system GTK directly, maps a real window for smoke
qualification, and mimics the consumer's version/config interface without its
dependency build cost. It does not replace full consumer qualification.
