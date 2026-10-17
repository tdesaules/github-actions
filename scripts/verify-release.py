#!/usr/bin/env python3
"""Fail closed on release tags until every Docker action uses a published digest."""
import hashlib
import json
import os
import re
import subprocess

from ci import ROOT, IMAGE, discover, git, mode
from images import ENGINE

SKOPEO = "quay.io/skopeo/stable:v1.22.3@sha256:7e735527d9d64c7456fc82d7f954c380b4a5dd1a8173115ce2ed7b3442ef36e0"
COSIGN = "ghcr.io/sigstore/cosign/cosign:v3.1.3@sha256:9e5c2f2edc34351160407ca3416c61855bdf9403c3c5936e0f0be7fc261611b8"


def main():
    mode("push", os.environ["GITHUB_REF"])
    subprocess.run(["git", "merge-base", "--is-ancestor", "HEAD", "origin/main"], check=True)
    actions, _ = discover()
    references = set()
    for directory in actions:
        ref = IMAGE.search((ROOT / directory / "action.yml").read_text())[1]
        if not re.search(r"@sha256:[a-f0-9]{64}$", ref):
            raise SystemExit(f"{directory}: bootstrap is not published/pinned. Publish both architectures, apply action-digests.patch, then create a NEW tag. Never move an existing tag.")
        references.add(ref)
    identity = f"https://github.com/{os.environ['GITHUB_REPOSITORY']}/.github/workflows/main.yml@refs/heads/main"
    for ref in sorted(references):
        raw = subprocess.check_output([ENGINE, "run", "--rm", SKOPEO, "inspect", "--raw", "docker://"+ref])
        if "sha256:"+hashlib.sha256(raw).hexdigest() != ref.split("@", 1)[1]:
            raise ValueError(f"digest mismatch: {ref}")
        manifests = json.loads(raw).get("manifests", [])
        if {m["platform"]["architecture"] for m in manifests} != {"amd64", "arm64"}:
            raise ValueError(f"both architectures required: {ref}")
        for target in [ref]+[ref.split("@", 1)[0]+"@"+m["digest"] for m in manifests]:
            subprocess.run([ENGINE, "run", "--rm", COSIGN, "verify", "--certificate-identity", identity, "--certificate-oidc-issuer", "https://token.actions.githubusercontent.com", target], check=True)
    print("Tag references and signatures verified. GitHub Release automation is not part of the OCI bootstrap milestone.")


if __name__ == "__main__":
    main()
