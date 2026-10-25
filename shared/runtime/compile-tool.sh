#!/bin/sh
# Rebuild pinned upstream source with explicitly selected dependency fixes.
set -eu
# Hosted module downloads repeatedly fail with HTTP/2 INTERNAL_ERROR from the
# proxy/checksum service. Scope HTTP/1.1 to this build process; retain TLS and
# Go checksum verification.
export GODEBUG="${GODEBUG:+$GODEBUG,}http2client=0"
mkdir -p /runtime/tmp
chmod 1777 /runtime/tmp
module=$1
version=$2
checksum=$3
command=$4
shift 4
if ! go mod download -json "$module@$version" > /tmp/module.json; then
  cat /tmp/module.json >&2
  exit 1
fi
grep -F "\"Sum\": \"$checksum\"" /tmp/module.json
mkdir -p /tool-source
cp -R "$(go env GOPATH)/pkg/mod/$module@$version/." /tool-source/
chmod -R u+w /tool-source
cd /tool-source
target="./cmd/$command"
if [ "$command" = registry ]; then
  # Build the untouched registry source as a versioned dependency so its real
  # upstream version remains visible in Go build information and the SBOM.
  mkdir -p /versioned-tool
  cd /versioned-tool
  go mod init github.com/tdesaules/registry-build
  go mod edit "-require=$module@$version"
  target="$module/cmd/$command"
fi
for requirement in "$@"; do
  go mod edit "-require=$requirement"
done
if [ "$command" = buildah ] || [ "$command" = skopeo ] || [ "$command" = cosign ]; then
  go mod edit -require=google.golang.org/grpc@v1.83.2
fi
if [ "$command" = buildah ] || [ "$command" = skopeo ]; then
  # New securejoin/go-archive APIs require their callers to move in lockstep.
  go mod edit -require=go.podman.io/storage@v1.64.2
fi
if [ "$command" = buildah ]; then
  go mod edit -require=github.com/fsouza/go-dockerclient@v1.13.3
fi
linkflags=''
if [ "$command" = registry ]; then
  linkflags="-X github.com/distribution/distribution/v3/version.version=$version"
fi
cgo=0
if [ "$command" = buildah ]; then
  # Namespace setup uses a C constructor before Go starts; disabling CGO
  # otherwise makes the reexec path recurse without entering the namespace.
  cgo=1
  linkflags='-linkmode external -extldflags -static'
fi
CGO_ENABLED=$cgo go build -p 2 -tags "${BUILDTAGS:-}" -mod=mod -trimpath -ldflags="-s -w $linkflags -X main.version=${version#v} -X main.gitDescription=$version -X sigs.k8s.io/release-utils/version.gitVersion=$version" -o /tool "$target"
