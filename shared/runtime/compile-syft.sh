#!/bin/sh
set -eu
sh shared/runtime/compile-tool.sh github.com/anchore/syft v1.54.1 h1:IdNS5Ha7WfatXHDL0qzjdf7DBVnfb8OI3FpSbltv0nc= syft golang.org/x/net@v0.60.0
