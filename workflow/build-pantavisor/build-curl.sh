#!/bin/sh
# Build the Debian source package against Trixie, retaining both TLS ABIs and
# real dpkg ownership/version records. No unstable binary packages are used.
set -eu
mkdir -p /curl-source /packages
cd /curl-source
curl --fail --location --proto '=https' --tlsv1.2 \
    https://deb.debian.org/debian/pool/main/c/curl/curl_8.23.0.orig.tar.gz -o curl.tar.gz
curl --fail --location --proto '=https' --tlsv1.2 \
    https://deb.debian.org/debian/pool/main/c/curl/curl_8.23.0-1.debian.tar.xz -o debian.tar.xz
echo 'e3de77cfa6367083c776a26647fc9f633b538c06c3baa73d931a252ac8f72b49  curl.tar.gz' | sha256sum -c -
echo '8f24b0c09bde25bb4c2fab6ec177336aa60ac73f737d5c06c93841241fa058d8  debian.tar.xz' | sha256sum -c -
mkdir source
tar -xf curl.tar.gz -C source --strip-components=1
tar -xf debian.tar.xz -C source
cd source
# Identify the local backport without changing the upstream curl version.
sed -i '1s/8.23.0-1/8.23.0-1~pantavisor1/; 1s/unstable/trixie/' debian/changelog
dpkg-source --before-build .
DEB_BUILD_OPTIONS='nocheck parallel=2' dpkg-buildpackage -B -us -uc -Pnocheck
for package in curl libcurl4t64 libcurl4-gnutls libcurl3t64-gnutls; do
    cp ../"${package}"_8.23.0-1~pantavisor1_*.deb /packages/
done
# Check generated package identities, not filenames alone.
for package in /packages/*.deb; do
    test "$(dpkg-deb -f "$package" Version)" = '8.23.0-1~pantavisor1'
done
