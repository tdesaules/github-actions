#!/bin/sh
# Preserve the Python 3.13 ABI and Debian layout while applying upstream fixes.
set -eu
mkdir -p /python-source /packages
cd /python-source
curl --fail --location --proto '=https' --tlsv1.2 \
    https://deb.debian.org/debian/pool/main/p/python3.13/python3.13_3.13.18.orig.tar.xz -o python.tar.xz
curl --fail --location --proto '=https' --tlsv1.2 \
    https://deb.debian.org/debian/pool/main/p/python3.13/python3.13_3.13.18-1.debian.tar.xz -o debian.tar.xz
echo 'abb0102e6d4ae027509bcb82327d9dd85515b44a3ba8ad538cfc5e9847029f68  python.tar.xz' | sha256sum -c -
echo '8f12d82b3cedd95970133622a48a3c3309a3ce8560e5c026d3438d3aecc8ef8b  debian.tar.xz' | sha256sum -c -
mkdir source
tar -xf python.tar.xz -C source --strip-components=1
tar -xf debian.tar.xz -C source
cd source
sed -i '1s/3.13.18-1/3.13.18-1~pantavisor1/; 1s/unstable/trixie/' debian/changelog
dpkg-source --before-build .
# PGO/LTO are build optimizations, not security features; keep native builds
# bounded on hosted runners. Targeted compatibility tests run on the final image.
DEB_BUILD_OPTIONS='nocheck nopgo nolto parallel=2' dpkg-buildpackage -B -us -uc -Pnocheck
for package in python3.13 python3.13-minimal libpython3.13-minimal libpython3.13-stdlib libpython3.13; do
    cp ../"${package}"_3.13.18-1~pantavisor1_*.deb /packages/
done
for package in /packages/*.deb; do
    test "$(dpkg-deb -f "$package" Version)" = '3.13.18-1~pantavisor1'
done
