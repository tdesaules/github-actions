#!/bin/sh
# The 5.4.1 release contains the four upstream 2026 security fixes. Its detached
# GNU signature was checked during review; the exact archive is pinned here.
set -eu
export SOURCE_DATE_EPOCH=1783616188
mkdir -p /gawk-source /packages /gawk-package/DEBIAN
cd /gawk-source
curl --fail --location --proto '=https' --tlsv1.2 \
    https://ftp.gnu.org/gnu/gawk/gawk-5.4.1.tar.xz -o source.tar.xz
echo '07f6f7342b7febe4313fc2c2542ad93d64fe20ad8717200109f105a826f5fd37  source.tar.xz' | sha256sum -c -
tar -xf source.tar.xz --strip-components=1
triplet=$(dpkg-architecture -qDEB_HOST_MULTIARCH)
# PMA needs process-personality changes unavailable in the action sandbox.
# KAS/BitBake use ordinary awk invocations, never persistent-memory programs.
./configure --prefix=/usr --libdir="/usr/lib/$triplet" --libexecdir="/usr/lib/$triplet" --disable-pma
make -j2
chown -R 30000:30000 /gawk-source
setpriv --reuid=30000 --regid=30000 --clear-groups make check
make DESTDIR=/gawk-package install
mkdir -p /gawk-package/usr/share/doc/gawk
cp COPYING /gawk-package/usr/share/doc/gawk/copyright
# Keep Debian's alternatives hooks so /usr/bin/awk resolves to the new gawk.
for script in postinst prerm; do
    cp "/var/lib/dpkg/info/gawk.$script" "/gawk-package/DEBIAN/$script"
done
mkdir debian
cat > debian/control <<'EOF'
Source: gawk
Maintainer: Pantavisor build tools <noreply@github.com>
Section: interpreters
Priority: optional

Package: gawk
Architecture: any
Description: GNU awk built from pinned upstream source for Pantavisor CI
EOF
set -- -e/gawk-package/usr/bin/gawk
for library in /gawk-package/usr/lib/"$triplet"/gawk/*.so; do
    [ ! -f "$library" ] || set -- "$@" "-e$library"
done
depends=$(dpkg-shlibdeps -O "$@")
depends=${depends#shlibs:Depends=}
cat > /gawk-package/DEBIAN/control <<EOF
Package: gawk
Source: gawk
Version: 1:5.4.1-0~pantavisor1
Architecture: $(dpkg --print-architecture)
Maintainer: Pantavisor build tools <noreply@github.com>
Section: interpreters
Priority: optional
Provides: awk
Depends: $depends
Description: GNU awk built from pinned upstream source for Pantavisor CI
EOF
cd /gawk-package
find usr -type f -exec md5sum '{}' + > DEBIAN/md5sums
dpkg-deb --root-owner-group --build /gawk-package /packages/gawk.deb
