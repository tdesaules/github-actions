#!/bin/sh
# Upgrade the XML library and the only direct runtime consumer as one ABI unit.
set -eu
mkdir -p /xml-source /packages
cd /xml-source
fetch() {
    curl --fail --location --proto '=https' --tlsv1.2 "$1" -o "$2"
    echo "$3  $2" | sha256sum -c -
}
fetch https://deb.debian.org/debian/pool/main/libx/libxml2/libxml2_2.15.4+dfsg.orig.tar.xz xml.tar.xz \
    5868d20db42ee21e4881bc53050c7fa226286a277cb3d0543f76b81f7571e934
fetch https://deb.debian.org/debian/pool/main/libx/libxml2/libxml2_2.15.4+dfsg-1.debian.tar.xz xml-debian.tar.xz \
    a125823450895c0785596a6fce26931ab0a369c82da4d6cfa6bbdac782ed9010
fetch https://deb.debian.org/debian/pool/main/libx/libxml-libxml-perl/libxml-libxml-perl_2.0207+dfsg+really+2.0134.orig.tar.xz perl.tar.xz \
    a27332a4a8ea4103fb0562bc8cf3aba4d28bdccb5331a59fd8fe2befb9e965fb
fetch https://deb.debian.org/debian/pool/main/libx/libxml-libxml-perl/libxml-libxml-perl_2.0207+dfsg+really+2.0134-8.debian.tar.xz perl-debian.tar.xz \
    ae01161f82f1f7819c8b1a0e823d4c7888cc579241d0481d8ac2cbe4f5c1e5cf
mkdir xml perl
tar -xf xml.tar.xz -C xml --strip-components=1
tar -xf xml-debian.tar.xz -C xml
tar -xf perl.tar.xz -C perl --strip-components=1
tar -xf perl-debian.tar.xz -C perl
cd xml
sed -i '1s/2.15.4+dfsg-1/2.15.4+dfsg-1~pantavisor1/; 1s/unstable/trixie/' debian/changelog
dpkg-source --before-build .
DEB_BUILD_OPTIONS='parallel=2' dpkg-buildpackage -B -us -uc -Pnopython
dpkg -i ../libxml2-16_2.15.4+dfsg-1~pantavisor1_*.deb ../libxml2-dev_2.15.4+dfsg-1~pantavisor1_*.deb
cp ../libxml2-16_2.15.4+dfsg-1~pantavisor1_*.deb /packages/
cd ../perl
sed -i '1s/2.0207+dfsg+really+2.0134-8/2.0207+dfsg+really+2.0134-8~pantavisor1/; 1s/unstable/trixie/' debian/changelog
# Trixie ships the Perl XS headers in libperl-dev, before the sid package split.
sed -i 's/perl-xs-dev/libperl-dev/g' debian/control
dpkg-source --before-build .
DEB_BUILD_OPTIONS='parallel=2' dpkg-buildpackage -B -us -uc
cp ../libxml-libxml-perl_2.0207+dfsg+really+2.0134-8~pantavisor1_*.deb /packages/
