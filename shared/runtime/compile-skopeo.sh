#!/bin/sh
set -eu
BUILDTAGS=containers_image_openpgp,exclude_graphdriver_btrfs,exclude_graphdriver_devicemapper sh shared/runtime/compile-tool.sh github.com/containers/skopeo v1.22.3 h1:r85TJYmaMhSgmtuPWxrsPQgaWL/+Q3y/cLLGE/DXjlc= skopeo golang.org/x/net@v0.60.0 golang.org/x/crypto@v0.56.0 golang.org/x/text@v0.41.0 google.golang.org/grpc@v1.83.2 go.opentelemetry.io/otel@v1.41.0 github.com/opencontainers/selinux@v1.13.0 github.com/sigstore/fulcio@v1.8.6
