#!/bin/sh
set -eu
sh shared/runtime/compile-tool.sh github.com/anchore/grype v0.120.1 h1:kaO5Vudd1jmjFNYJoOro7AIg10cjrKAy71ZaOFdX0js= grype golang.org/x/net@v0.60.0
