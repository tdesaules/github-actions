#!/bin/sh
# Run only in the upstream assembly stage, never in the final scratch image.
set -eu
root=/runtime
mkdir -p "$root/usr/bin" "$root/etc/containers" "$root/etc/ssl/certs" "$root/tmp" "$root/var/tmp" "$root/var/lib/containers" "$root/run/containers"
chmod 1777 "$root/tmp" "$root/var/tmp"
copy_binary() {
  binary=$(command -v "$1")
  target=${2:-/usr/bin/$1}
  mkdir -p "$root$(dirname "$target")"
  cp -L "$binary" "$root$target"
  # ldd may fail for static binaries. Copy only absolute resolved library paths.
  ldd "$binary" 2>/dev/null | awk '{ for (i=1;i<=NF;i++) if ($i ~ /^\//) print $i }' | while read -r library; do
    cp -L --parents "$library" "$root"
  done
}
copy_binary "$1"
if [ "$1" = buildah ]; then
  copy_binary crun
  copy_binary /usr/libexec/podman/netavark /usr/libexec/podman/netavark
  copy_binary /usr/libexec/podman/aardvark-dns /usr/libexec/podman/aardvark-dns
fi
cp -L /etc/ssl/certs/ca-certificates.crt "$root/etc/ssl/certs/ca-certificates.crt"
cp /etc/containers/policy.json "$root/etc/containers/policy.json"
printf '[registries.search]\nregistries = []\n' > "$root/etc/containers/registries.conf"
printf 'root:x:0:0:root:/tmp:/sbin/nologin\nnonroot:x:65532:65532:nonroot:/tmp:/sbin/nologin\n' > "$root/etc/passwd"
printf 'root:x:0:\nnonroot:x:65532:\n' > "$root/etc/group"
printf 'hosts: files dns\n' > "$root/etc/nsswitch.conf"
