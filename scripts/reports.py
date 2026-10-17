#!/usr/bin/env python3
"""Generate reports from the exact archive; pinned upstream tools break bootstrap cycles."""
import argparse
import json
import os
from pathlib import Path
import subprocess

from images import ENGINE, ROOT, architecture, mount, run

SYFT = "docker.io/anchore/syft:v1.54.1@sha256:3eb5379ba7b409c3f4069b686110527af0c47df993fa5c10d13e7cf34f49b1aa"
GRYPE = "docker.io/anchore/grype:v0.120.1@sha256:e4a44ef45d285b829ce6efe2642980329661bd2d18eab5fc539138d4adaebbbe"


def reports(directory):
    bind = mount(directory.resolve(), "/reports")
    run(ENGINE, "run", "--rm", "--user=0", "-v", bind, SYFT, "scan", "oci-archive:/reports/image.oci.tar", "-o", "spdx-json=/reports/sbom.spdx.json")
    # Save database metadata separately so reports remain attributable to a DB snapshot.
    command = [ENGINE, "run", "--rm", "--user=0", "-v", bind, "-e", "GRYPE_DB_CACHE_DIR=/reports/grype-db", GRYPE]
    run(*command, "db", "update")
    status = subprocess.check_output(command+["db", "status", "-o", "json"])
    (directory / "grype-db.json").write_bytes(status)
    run(*command, "sbom:/reports/sbom.spdx.json", "--fail-on", os.environ.get("FAIL_ON", "high"), "-o", "json", "--file", "/reports/grype.json")


if __name__ == "__main__":
    parser = argparse.ArgumentParser()
    parser.add_argument("image")
    args = parser.parse_args()
    reports(ROOT / ".build" / args.image / architecture())
