#!/usr/bin/env python3
"""Publish tested bootstrap archives; never build, force-push, or invent digests."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import re
import subprocess

from ci import ROOT, IMAGE, SEMVER, aggregate, discover
from images import ENGINE, container, local_ref, action_ref, load_archive, mount, run


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
    if not SEMVER.fullmatch(args.version) or '+' in args.version:
        raise ValueError('version must be SemVer without v or registry-incompatible build metadata')
    if args.version != '0.0.0-bootstrap' and not args.sign:
        raise ValueError('catalogue releases require signatures')
    directory = args.directory.resolve()
    paths = sorted(directory.glob("**/metadata.json"))
    results = aggregate(paths)
    revision = subprocess.check_output(['git', 'rev-parse', 'HEAD'], cwd=ROOT, text=True).strip()
    if any(r.get('revision') != revision for r in results):
        raise ValueError('publication artifacts must come from this exact source revision')
    if len({r['version'] for r in results}) > 1:
        raise ValueError('publication artifacts mix candidate versions')
    _, owners = discover()
    changed = {r['image'] for r in results}
    if args.version == '0.0.0-bootstrap' and changed != set(owners):
        raise ValueError("bootstrap publication requires all images on both architectures")
    locations = {(json.loads(p.read_text())["image"], json.loads(p.read_text())["arch"]): p.parent for p in paths}
    for image in ('oci-publish-image', 'oci-merge-image-index', 'oci-new-image-signature', 'oci-test-image-signature'):
        if image in changed:
            load_archive(locations[(image, "amd64")] / "image.oci.tar", local_ref(image, args.version))
        else:
            reference = IMAGE.search((ROOT / owners[image] / 'action.yml').read_text())[1]
            if '@sha256:' not in reference:
                raise ValueError(f'{image}: missing tested image or published immutable reference')
            run(ENGINE, 'pull', reference)
            run(ENGINE, 'tag', reference, local_ref(image, args.version))
    token = os.environ["GHCR_TOKEN"]
    username = os.environ["GHCR_USERNAME"]
    common = {"registry": "ghcr.io", "registry-username": username, "registry-password": token}

    def call(operation, inputs):
        extra = []
        if operation == 'new-image-signature':
            extra = ["-e", "ACTIONS_ID_TOKEN_REQUEST_URL", "-e", "ACTIONS_ID_TOKEN_REQUEST_TOKEN"]
        container(action_ref(operation, args.version), operation, directory, common | inputs, extra)

    # Use an isolated auth file. Its contents never appear in command logs or artifacts.
    import base64
    import tempfile
    with tempfile.TemporaryDirectory(prefix="publish-auth-") as tmp:
        auth = Path(tmp) / "auth.json"
        auth.write_text(json.dumps({"auths": {"ghcr.io": {"auth": base64.b64encode(f"{username}:{token}".encode()).decode()}}}))
        auth.chmod(0o600)

        def raw(reference, missing=False):
            command = [ENGINE, "run", "--rm", "--entrypoint=/usr/bin/skopeo", "-v", mount(directory, "/github/workspace"), "-v", mount(auth, "/auth.json", True), "-e", "REGISTRY_AUTH_FILE=/auth.json", action_ref('publish-image', args.version), "inspect", "--raw", reference]
            result = subprocess.run(command, capture_output=True)
            if result.returncode:
                text = result.stderr.decode()
                if missing and ("manifest unknown" in text.lower() or "name unknown" in text.lower()):
                    return None
                raise RuntimeError(text)
            return result.stdout

        state = {"version": args.version, "images": {}}
        for image in sorted(owners):
            if image not in changed:
                reference = IMAGE.search((ROOT / owners[image] / 'action.yml').read_text())[1]
                if '@sha256:' not in reference:
                    raise ValueError(f'{image}: unchanged image is not pinned')
                index = raw('docker://'+reference)
                if 'sha256:'+hashlib.sha256(index).hexdigest() != reference.split('@')[1]:
                    raise ValueError('reused index digest mismatch')
                children = {m['platform']['architecture']: m['digest'] for m in json.loads(index)['manifests']}
                if set(children) != {'amd64', 'arm64'}:
                    raise ValueError('reused image is missing an architecture')
                identity = f"https://github.com/{os.environ['GITHUB_REPOSITORY']}/.github/workflows/main.yml@refs/heads/main"
                for value in [reference, *[reference.split('@')[0]+'@'+d for d in children.values()]]:
                    call('test-image-signature', {'image': value, 'identity': identity, 'issuer': 'https://token.actions.githubusercontent.com', 'output': f'signatures/{image}-{value.split(":")[-1]}.json'})
                state['images'][image] = {'reference': reference, 'platforms': children, 'signed': True, 'reused': True}
                (directory / 'publication.json').write_text(json.dumps(state, indent=2)+'\n')
                continue
            repository = f"ghcr.io/{args.owner}/github-actions/{image}"
            archives = []
            child_digests = {}
            for arch in ("amd64", "arm64"):
                relative = str((locations[(image, arch)] / "image.oci.tar").relative_to(directory))
                archives.append(relative)
                expected = "sha256:"+hashlib.sha256(raw("oci-archive:/github/workspace/"+relative)).hexdigest()
                target = repository+'@'+expected
                existing = raw("docker://"+target, missing=True)
                actual = None if existing is None else "sha256:"+hashlib.sha256(existing).hexdigest()
                if not check_existing(expected, actual, target):
                    call("publish-image", {"source": "oci-archive:"+relative, "destination": "docker://"+target})
                published = "sha256:"+hashlib.sha256(raw("docker://"+target)).hexdigest()
                if published != expected:
                    raise ValueError(f"published digest changed for {target}")
                child_digests[arch] = published
            manifest_path = 'indexes/'+image+'.json'
            call('merge-image-index', {'sources': json.dumps(archives), 'destination': 'docker://'+repository, 'output': manifest_path})
            expected_index = (directory / manifest_path).read_bytes()
            target = repository+'@sha256:'+hashlib.sha256(expected_index).hexdigest()
            index = raw("docker://"+target)
            if index != expected_index:
                raise ValueError('published index differs from the assembled manifest')
            children = {m["platform"]["architecture"]: m["digest"] for m in json.loads(index)["manifests"]}
            if children != child_digests:
                raise ValueError(f"index does not contain the exact tested children: {target}")
            digest = "sha256:"+hashlib.sha256(index).hexdigest()
            if args.sign:
                identity = f"https://github.com/{os.environ['GITHUB_REPOSITORY']}/.github/workflows/main.yml@refs/heads/main"
                for value in [*child_digests.values(), digest]:
                    reference = repository+"@"+value
                    call("new-image-signature", {"image": reference})
                    call("test-image-signature", {"image": reference, "identity": identity, "issuer": "https://token.actions.githubusercontent.com", "output": f"signatures/{image}-{value[7:]}.json"})
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
