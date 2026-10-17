#!/usr/bin/env python3
"""Rescan both published variants, updating Grype's DB explicitly."""
import json
import subprocess

from ci import ROOT, IMAGE, discover
from images import ENGINE, mount, run
from reports import SYFT, GRYPE


def main():
    _, owners = discover()
    failures = []
    for image, directory in sorted(owners.items()):
        reference = IMAGE.search((ROOT / directory / "action.yml").read_text())[1]
        if "@sha256:" not in reference:
            print(f"{image}: not published yet; no mutable bootstrap tag will be scanned")
            continue
        for arch in ("amd64", "arm64"):
            out = ROOT / ".build/maintenance" / image / arch
            out.mkdir(parents=True, exist_ok=True)
            bind = mount(out.resolve(), "/reports")
            try:
                run(ENGINE, "run", "--rm", "--user=0", "-v", bind, SYFT, "scan", "registry:"+reference, "--platform", "linux/"+arch, "-o", "spdx-json=/reports/sbom.spdx.json")
                command = [ENGINE, "run", "--rm", "--user=0", "-v", bind, "-e", "GRYPE_DB_CACHE_DIR=/reports/db", GRYPE]
                run(*command, "db", "update")
                (out / "database.json").write_bytes(subprocess.check_output(command+["db", "status", "-o", "json"]))
                run(*command, "sbom:/reports/sbom.spdx.json", "--fail-on", "high", "-o", "json", "--file", "/reports/grype.json")
            except subprocess.CalledProcessError:
                failures.append(image+"/"+arch)
    if failures:
        raise SystemExit("Maintenance scans failed: "+", ".join(failures))


if __name__ == "__main__":
    main()
