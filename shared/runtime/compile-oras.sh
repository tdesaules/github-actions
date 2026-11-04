#!/bin/sh
# Preserve the upstream ORAS version while rebuilding its verified source with
# the catalogue's Go toolchain and an explicit crypto dependency fix.
set -eu
sh /compile-tool.sh oras.land/oras v1.3.4 h1:ny44PJSsrLalEmyntFRi3Tmy3b1PTUZ3S4FeIRdVcU4= oras golang.org/x/crypto@v0.56.0
mkdir -p /runtime/usr/share/licenses/oras
cp /tool-source/LICENSE /runtime/usr/share/licenses/oras/LICENSE
