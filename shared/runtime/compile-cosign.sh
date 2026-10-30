#!/bin/sh
set -eu
sh shared/runtime/compile-tool.sh github.com/sigstore/cosign/v3 v3.1.3 h1:001JQRI/PJ/5T+g/kJ1KTvKFbb322+fomc+pHDZ/6sg= cosign golang.org/x/net@v0.60.0 golang.org/x/crypto@v0.56.0 golang.org/x/text@v0.41.0 golang.org/x/mod@v0.40.0 google.golang.org/grpc@v1.83.1
