#!/usr/bin/env python3
"""Rescan both published variants, updating Grype's DB explicitly."""
import subprocess

from ci import ROOT, IMAGE, discover
from reports import reports


def main():
    _, owners = discover()
    failures = []
    database_directory = ROOT / '.build/maintenance-db'
    update_database = True
    for image, directory in sorted(owners.items()):
        reference = IMAGE.search((ROOT / directory / "action.yml").read_text())[1]
        if "@sha256:" not in reference:
            print(f"{image}: not published yet; no mutable bootstrap tag will be scanned")
            continue
        for arch in ("amd64", "arm64"):
            out = ROOT / ".build/maintenance" / image / arch
            out.mkdir(parents=True, exist_ok=True)
            try:
                reports(out, 'registry:'+reference, 'linux/'+arch, database_directory, update_database)
                update_database = False
            except subprocess.CalledProcessError:
                failures.append(image+"/"+arch)
    if failures:
        raise SystemExit("Maintenance scans failed: "+", ".join(failures))


if __name__ == "__main__":
    main()
