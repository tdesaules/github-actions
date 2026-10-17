#!/usr/bin/env python3
"""Publish tested bootstrap archives; never build, force-push, or invent digests."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import re
import subprocess

from ci import ROOT, IMAGE, aggregate, discover
from images import ENGINE, container, local_ref, load_archive, mount, run


def check_existing(expected, actual, reference):
    if actual is not None and expected != actual:
        raise ValueError(f"refusing to overwrite {reference}: existing digest {actual}, candidate {expected}")
    return actual == expected


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("directory", type=Path)
    parser.add_argument("--owner", required=True)
    parser.add_argument("--version", default="0.0.0-bootstrap")
    parser.add_argument("--sign", action="store_true")
    args = parser.parse_args()
    if not re.fullmatch(r"[a-z0-9][a-z0-9-]*", args.owner):
        raise ValueError("owner must be lowercase GitHub owner")
    if args.version != "0.0.0-bootstrap":
        raise ValueError("this first-lot publisher only publishes 0.0.0-bootstrap; semantic release preparation is a subsequent milestone")
    directory = args.directory.resolve()
    paths = sorted(directory.glob("**/metadata.json"))
    results = aggregate(paths)
    _, owners = discover()
    if {r["image"] for r in results} != set(owners):
        raise ValueError("bootstrap publication requires all images on both architectures")
    locations = {(json.loads(p.read_text())["image"], json.loads(p.read_text())["arch"]): p.parent for p in paths}
    for image in ("skopeo", "buildah", "cosign"):
        load_archive(locations[(image, "amd64")] / "image.oci.tar", local_ref(image, args.version))
    token = os.environ["GHCR_TOKEN"]
    username = os.environ["GHCR_USERNAME"]
    common = {"registry": "ghcr.io", "registry-username": username, "registry-password": token}

    def call(tool, operation, inputs):
        extra = []
        if tool == "cosign":
            extra = ["-e", "ACTIONS_ID_TOKEN_REQUEST_URL", "-e", "ACTIONS_ID_TOKEN_REQUEST_TOKEN"]
        container(local_ref(tool, args.version), operation, directory, common | inputs, extra)

    # Use an isolated auth file. Its contents never appear in command logs or artifacts.
    import base64
    import tempfile
    with tempfile.TemporaryDirectory(prefix="publish-auth-") as tmp:
        auth = Path(tmp) / "auth.json"
        auth.write_text(json.dumps({"auths": {"ghcr.io": {"auth": base64.b64encode(f"{username}:{token}".encode()).decode()}}}))
        auth.chmod(0o600)

        def raw(reference, missing=False):
            command = [ENGINE, "run", "--rm", "--entrypoint=/usr/bin/skopeo", "-v", mount(directory, "/github/workspace"), "-v", mount(auth, "/auth.json", True), "-e", "REGISTRY_AUTH_FILE=/auth.json", local_ref("skopeo", args.version), "inspect", "--raw", reference]
            result = subprocess.run(command, capture_output=True)
            if result.returncode:
                text = result.stderr.decode()
                if missing and ("manifest unknown" in text.lower() or "name unknown" in text.lower()):
                    return None
                raise RuntimeError(text)
            return result.stdout

        state = {"version": args.version, "images": {}}
        for image in sorted(owners):
            repository = f"ghcr.io/{args.owner}/github-actions/{image}"
            archives = []
            child_digests = {}
            for arch in ("amd64", "arm64"):
                relative = str((locations[(image, arch)] / "image.oci.tar").relative_to(directory))
                archives.append(relative)
                expected = "sha256:"+hashlib.sha256(raw("oci-archive:/github/workspace/"+relative)).hexdigest()
                target = f"{repository}:{args.version}-{arch}"
                existing = raw("docker://"+target, missing=True)
                actual = None if existing is None else "sha256:"+hashlib.sha256(existing).hexdigest()
                if not check_existing(expected, actual, target):
                    call("skopeo", "publish-image", {"source": "oci-archive:"+relative, "destination": "docker://"+target})
                published = "sha256:"+hashlib.sha256(raw("docker://"+target)).hexdigest()
                if published != expected:
                    raise ValueError(f"published digest changed for {target}")
                child_digests[arch] = published
            target = f"{repository}:{args.version}"
            existing = raw("docker://"+target, missing=True)
            if existing is not None:
                manifests = json.loads(existing).get("manifests", [])
                existing_children = {m["platform"]["architecture"]: m["digest"] for m in manifests}
                if existing_children != child_digests:
                    raise ValueError(f"refusing to replace existing multiarchitecture tag {target}")
            else:
                call("buildah", "merge-image-index", {"sources": json.dumps(archives), "destination": "docker://"+target})
            index = raw("docker://"+target)
            children = {m["platform"]["architecture"]: m["digest"] for m in json.loads(index)["manifests"]}
            if children != child_digests:
                raise ValueError(f"index does not contain the exact tested children: {target}")
            digest = "sha256:"+hashlib.sha256(index).hexdigest()
            if args.sign:
                identity = f"https://github.com/{os.environ['GITHUB_REPOSITORY']}/.github/workflows/main.yml@refs/heads/main"
                for value in [*child_digests.values(), digest]:
                    reference = repository+"@"+value
                    call("cosign", "new-image-signature", {"image": reference})
                    call("cosign", "test-image-signature", {"image": reference, "identity": identity, "issuer": "https://token.actions.githubusercontent.com", "output": f"signatures/{image}-{value[7:]}.json"})
            state["images"][image] = {"reference": repository+"@"+digest, "platforms": child_digests, "signed": args.sign}
            (directory / "publication.json").write_text(json.dumps(state, indent=2)+"\n")
        # Only after all images are published and verified, generate a reviewable patch.
        import difflib
        patch = []
        actions, _ = discover()
        for action, image in sorted(actions.items()):
            path = ROOT / action / "action.yml"
            old = path.read_text()
            new = IMAGE.sub(lambda m: m[0].replace(m[1], state["images"][image]["reference"]), old)
            patch.extend(difflib.unified_diff(old.splitlines(True), new.splitlines(True), fromfile="a/"+str(path.relative_to(ROOT)), tofile="b/"+str(path.relative_to(ROOT))))
        (directory / "action-digests.patch").write_text("".join(patch))


if __name__ == "__main__":
    main()
