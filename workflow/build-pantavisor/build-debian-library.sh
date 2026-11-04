#!/bin/sh
# Backport checksum-pinned Debian sources without importing unstable binaries.
# Arguments: source URL/hash, packaging URL/hash, then exported binary packages.
set -eu
source_url=$1
source_hash=$2
packaging_url=$3
packaging_hash=$4
shift 4
mkdir -p /backport-source /packages
cd /backport-source
curl --fail --location --proto '=https' --tlsv1.2 "$source_url" -o upstream.tar
curl --fail --location --proto '=https' --tlsv1.2 "$packaging_url" -o debian.tar
echo "$source_hash  upstream.tar" | sha256sum -c -
echo "$packaging_hash  debian.tar" | sha256sum -c -
mkdir source
tar -xf upstream.tar -C source --strip-components=1
tar -xf debian.tar -C source
cd source
original=$(dpkg-parsechangelog -S Version)
version="$original${BACKPORT_SUFFIX:-~pantavisor1}"
python3 - "$original" "$version" <<'PY'
from pathlib import Path
import sys
p = Path('debian/changelog')
lines = p.read_text().splitlines(keepends=True)
assert '(' + sys.argv[1] + ')' in lines[0]
lines[0] = lines[0].replace('(' + sys.argv[1] + ')', '(' + sys.argv[2] + ')', 1).replace(' unstable;', ' trixie;', 1)
p.write_text(''.join(lines))
PY
if [ -n "${SECURITY_PATCH_URL:-}" ]; then
    test -n "${SECURITY_PATCH_SHA256:-}"
    mkdir -p debian/patches
    curl --fail --location --proto '=https' --tlsv1.2 "$SECURITY_PATCH_URL" -o debian/patches/pantavisor-security.patch
    echo "$SECURITY_PATCH_SHA256  debian/patches/pantavisor-security.patch" | sha256sum -c -
    printf '\npantavisor-security.patch\n' >> debian/patches/series
fi
if [ -n "${LOCAL_SECURITY_PATCH:-}" ]; then
    test -n "${LOCAL_SECURITY_PATCH_SHA256:-}"
    mkdir -p debian/patches
    cp "$LOCAL_SECURITY_PATCH" debian/patches/pantavisor-bounds.patch
    echo "$LOCAL_SECURITY_PATCH_SHA256  debian/patches/pantavisor-bounds.patch" | sha256sum -c -
    printf '\npantavisor-bounds.patch\n' >> debian/patches/series
fi
dpkg-source --before-build .
chown -R 30000:30000 /backport-source
HOME=/tmp DEB_BUILD_OPTIONS='parallel=2' setpriv --reuid=30000 --regid=30000 --clear-groups dpkg-buildpackage -B -us -uc
for package in "$@"; do
    cp ../"${package}_${version#*:}_"*.deb /packages/
done
for package in /packages/*.deb; do
    test "$(dpkg-deb -f "$package" Version)" = "$version"
done
