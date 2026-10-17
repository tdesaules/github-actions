#!/bin/sh
set -eu
cd /github/workspace
mkdir -p .build/qualification
printf 'FROM docker.io/library/golang@sha256:f92b6ef800e499660581efdabdf25d9d817a9d124eaf900924f0504e7e27e12d\nRUN printf "namespace-test\\n" > /proof\n' > .build/qualification/Containerfile
buildah --storage-driver=vfs bud --isolation=chroot --network=host -f .build/qualification/Containerfile -t localhost/qualification .build/qualification
buildah --storage-driver=vfs push localhost/qualification oci-archive:/github/workspace/.build/qualification/proof.tar
