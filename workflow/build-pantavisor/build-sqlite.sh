#!/bin/sh
# Backport the C library with its original SONAME and Debian feature flags.
set -eu
mkdir -p /sqlite-source /packages
cd /sqlite-source
fetch() {
    curl --fail --location --proto '=https' --tlsv1.2 "$1" -o "$2"
    echo "$3  $2" | sha256sum -c -
}
fetch https://deb.debian.org/debian/pool/main/s/sqlite3/sqlite3_3.54.1.orig.tar.xz sqlite.tar.xz \
    faeff7682200b3dd58c8f0f229892a7271c73c1fdabc86e7315eb31eb7cb6e83
fetch https://deb.debian.org/debian/pool/main/s/sqlite3/sqlite3_3.54.1.orig-www.tar.xz www.tar.xz \
    67906ee141109bebe43cdcf197892806397dabc79f5cd7f05f1278f47b0c5e25
fetch https://deb.debian.org/debian/pool/main/s/sqlite3/sqlite3_3.54.1-1.debian.tar.xz debian.tar.xz \
    3f05602b0f4e1441b88f92e9afb2980e148659842565b1f1e27012988433a1d9
mkdir source
tar -xf sqlite.tar.xz -C source --strip-components=1
tar -xf debian.tar.xz -C source
mkdir source/www
tar -xf www.tar.xz -C source/www --strip-components=1
cd source
sed -i '1s/3.54.1-1/3.54.1-1~pantavisor1/; 1s/unstable/trixie/' debian/changelog
# Trixie uses Tcl 8.6. No Tcl bindings are exported from this builder.
sed -i 's/, tcl9.1-dev//g' debian/control
sed -i 's/^TCLV_SECONDARY = 9.1/TCLV_SECONDARY =/; /TCLV_SECONDARY).*install-tcl/d' debian/rules
dpkg-source --before-build .
DEB_BUILD_OPTIONS='parallel=2' dpkg-buildpackage -B -us -uc
# Permission/readonly tests must run with the same non-root model as KAS.
chown -R 30000:30000 /sqlite-source
NJOB=2 setpriv --reuid=30000 --regid=30000 --clear-groups make -C debian/8.6 quicktest
cp ../libsqlite3-0_3.54.1-1~pantavisor1_*.deb /packages/
