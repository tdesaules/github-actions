#!/bin/sh
# Assemble only requested executables and their resolved shared libraries.
set -eu
mkdir -p /runtime/tmp /runtime/etc/ssl/certs
chmod 1777 /runtime/tmp
for binary in "$@"; do
    mkdir -p "/runtime$(dirname "$binary")"
    cp -L "$binary" "/runtime$binary"
    # Static executables have no dynamic dependencies.
    ldd "$binary" 2>/dev/null | awk '{for (i=1;i<=NF;i++) if ($i ~ /^\//) print $i}' > /libraries
    while read -r library; do
        mkdir -p "/runtime$(dirname "$library")"
        cp -L "$library" "/runtime$library"
    done < /libraries
done
cp /etc/ssl/certs/ca-certificates.crt /runtime/etc/ssl/certs/ca-certificates.crt
printf 'root:x:0:0:root:/tmp:/bin/sh\nnonroot:x:65532:65532:nonroot:/tmp:/bin/sh\n' > /runtime/etc/passwd
printf 'root:x:0:\nnonroot:x:65532:\n' > /runtime/etc/group
printf 'hosts: files dns\n' > /runtime/etc/nsswitch.conf
