#!/usr/bin/env python3
"""Generate reports from the exact archive; pinned upstream tools break bootstrap cycles."""
import argparse
import json
import os
from pathlib import Path
import subprocess

from images import ENGINE, ROOT, architecture, mount, run
from scan_review import review

SYFT = "docker.io/anchore/syft:v1.54.1@sha256:3eb5379ba7b409c3f4069b686110527af0c47df993fa5c10d13e7cf34f49b1aa"
GRYPE = "docker.io/anchore/grype:v0.120.1@sha256:e4a44ef45d285b829ce6efe2642980329661bd2d18eab5fc539138d4adaebbbe"


def reports(directory, source='oci-archive:/reports/image.oci.tar', platform=None):
    bind = mount(directory.resolve(), "/reports")
    platform_args = ['--platform', platform] if platform else []
    run(ENGINE, "run", "--rm", "--user=0", "-v", bind, '-e', 'SYFT_GOLANG_CAPTURE_SYMBOLS=all', SYFT, "scan", source, *platform_args, "-o", "spdx-json=/reports/sbom.spdx.json", '-o', 'syft-json=/reports/sbom.syft.json')
    # Save database metadata separately so reports remain attributable to a DB snapshot.
    command = [ENGINE, "run", "--rm", "--user=0", "-v", bind, "-e", "GRYPE_DB_CACHE_DIR=/reports/grype-db", GRYPE]
    run(*command, "db", "update")
    status = subprocess.check_output(command+["db", "status", "-o", "json"])
    (directory / "grype-db.json").write_bytes(status)
    configuration, evidence = review(json.loads((directory / 'sbom.syft.json').read_text()),
                                     json.loads((ROOT / 'grype-review.json').read_text()))
    # JSON is a YAML subset. The report retains ignoredMatches and their reason.
    (directory / 'grype-reviewed.json').write_text(json.dumps(configuration, indent=2)+'\n')
    (directory / 'review-evidence.json').write_text(json.dumps(evidence, indent=2)+'\n')
    run(*command, "sbom:/reports/sbom.syft.json", '--config', '/reports/grype-reviewed.json', "--fail-on", os.environ.get("FAIL_ON", "high"), "-o", "json", "--file", "/reports/grype.json")


if __name__ == "__main__":
    parser = argparse.ArgumentParser()
    parser.add_argument("image")
    args = parser.parse_args()
    reports(ROOT / ".build" / args.image / architecture())
