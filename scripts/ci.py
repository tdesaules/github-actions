#!/usr/bin/env python3
"""Discover components from actual files; select work without a central catalogue."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import re
import subprocess

from oci_archive import inspect_archive

ROOT = Path(__file__).resolve().parents[1]
SEMVER = re.compile(r"^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)(?:-((?:0|[1-9]\d*|\d*[A-Za-z-][0-9A-Za-z-]*)(?:\.(?:0|[1-9]\d*|\d*[A-Za-z-][0-9A-Za-z-]*))*))?(?:\+([0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*))?$")
IMAGE = re.compile(r"^(?:  image|        BUILDER_IMAGE): docker://([^\s]+)$", re.M)


def git(*args):
    return subprocess.check_output(["git", *args], cwd=ROOT).decode().strip()


def discover(root=ROOT):
    actions = {}
    owners = {}
    for path in sorted(root.glob("oci/*/action.yml")):
        match = IMAGE.search(path.read_text())
        if not match:
            raise ValueError(f"{path}: missing Docker image reference")
        reference = match[1]
        image = reference.rsplit("/", 1)[1].split(":")[0].split("@")[0]
        directory = str(path.parent.relative_to(root))
        actions[directory] = image
        if (path.parent / "Containerfile").is_file():
            if image in owners:
                raise ValueError(f"duplicate Containerfile owner for {image}")
            owners[image] = directory
    if set(actions.values()) != set(owners):
        raise ValueError("every action image must have exactly one Containerfile owner")
    return actions, owners


def select(paths, full=False, root=ROOT):
    actions, owners = discover(root)
    images, selected_actions = set(), set()
    if full:
        images.update(owners)
        selected_actions.update(actions)
    for path in paths:
        if path.startswith(".github/workflows/") or path == 'grype-review.json':
            selected_actions.update(actions)
        if path == 'shared/run-builder.py':
            selected_actions.update(a for a, tool in actions.items() if tool == 'buildah')
        elif path.startswith(("shared/", "scripts/", "tasks/")) or path in ("go.mod", "go.sum", ".containerignore", ".dockerignore", "taskfile.yml"):
            images.update(owners)
            selected_actions.update(actions)
        for directory, image in actions.items():
            if path.startswith(directory + "/"):
                selected_actions.add(directory)
                filename = path[len(directory) + 1:]
                if filename == "Containerfile" or filename == ".containerignore" or filename.startswith(("src/", "dependencies.")):
                    images.add(image)
    for directory, image in actions.items():
        if image in images:
            selected_actions.add(directory)
    # Tests of an action-only change use its pinned image, not a rebuilt image.
    matrix = []
    for image in sorted({actions[a] for a in selected_actions} | images):
        for arch in ("amd64", "arm64"):
            matrix.append({"image": image, "directory": owners[image], "arch": arch,
                           "runner": "ubuntu-24.04" if arch == "amd64" else "ubuntu-24.04-arm",
                           "rebuild": image in images})
    if len(matrix) > 256:
        raise ValueError("GitHub supports at most 256 matrix jobs")
    return {"include": matrix}


def changed_paths(before, after):
    # --no-renames intentionally exposes BOTH names as delete/add; deleted owners
    # are absent from discovery and are therefore never scheduled for rebuilding.
    data = subprocess.check_output(["git", "diff", "--name-only", "--no-renames", "-z", before, after], cwd=ROOT)
    return [p.decode() for p in data.split(b"\0") if p]


def mode(event, ref):
    if event == "schedule":
        return "maintenance"
    if event == "workflow_dispatch":
        if ref != "refs/heads/main":
            raise ValueError("release preparation must target main")
        return "prepare"
    if ref.startswith("refs/tags/"):
        tag = ref.removeprefix("refs/tags/")
        if not SEMVER.fullmatch(tag):
            raise ValueError(f"tag is not SemVer without v: {tag}")
        return "release"
    return "development"


def aggregate(paths):
    result = {}
    for path in sorted(paths):
        item = json.loads(Path(path).read_text())
        key = (item["image"], item["arch"])
        if key in result:
            raise ValueError(f"duplicate matrix result: {key}")
        archive = Path(path).parent / "image.oci.tar"
        with archive.open("rb") as stream:
            actual = hashlib.file_digest(stream, "sha256").hexdigest()
        if actual != item["archive_sha256"]:
            raise ValueError(f"archive checksum mismatch: {archive}")
        if inspect_archive(archive)["arch"] != item["arch"]:
            raise ValueError(f"archive architecture does not match result: {archive}")
        if not item.get("tested"):
            raise ValueError(f"untested result: {path}")
        result[key] = item
    for image in {key[0] for key in result}:
        if {key[1] for key in result if key[0] == image} != {"amd64", "arm64"}:
            raise ValueError(f"both tested architectures required for {image}")
    return [result[key] for key in sorted(result)]


def emit(name, value):
    if not isinstance(value, str):
        value = json.dumps(value, separators=(",", ":"))
    print(f"{name}={value}")
    if os.environ.get("GITHUB_OUTPUT"):
        with open(os.environ["GITHUB_OUTPUT"], "a") as stream:
            stream.write(f"{name}={value}\n")


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("command", choices=("select", "validate", "aggregate"))
    parser.add_argument("paths", nargs="*")
    args = parser.parse_args()
    if args.command == "aggregate":
        print(json.dumps(aggregate(args.paths), indent=2))
        return
    actions, owners = discover()
    if args.command == "validate":
        for name, directory in owners.items():
            text = (ROOT / directory / "Containerfile").read_text()
            if "FROM scratch AS final" not in text:
                raise ValueError(f"{name}: final stage must use scratch")
            stages = set()
            for line in text.splitlines():
                if line.startswith('FROM '):
                    words = line.split()
                    if words[1] != 'scratch' and words[1] not in stages and '@sha256:' not in words[1]:
                        raise ValueError(f"{name}: unpinned base")
                    if len(words) == 4 and words[2].upper() == 'AS':
                        stages.add(words[3])
        print(f"Validated {len(actions)} Docker actions and {len(owners)} scratch image definitions")
        return
    event = os.environ.get("GITHUB_EVENT_NAME", "push")
    ref = os.environ.get("GITHUB_REF", "refs/heads/main")
    pipeline_mode = mode(event, ref)
    after = os.environ.get("GITHUB_SHA") or git("rev-parse", "HEAD")
    paths = args.paths
    full = pipeline_mode == "prepare"
    if pipeline_mode == "development" and not paths:
        payload = json.loads(Path(os.environ["GITHUB_EVENT_PATH"]).read_text()) if os.environ.get("GITHUB_EVENT_PATH") else {}
        before = payload.get("before", "")
        if not before or set(before) == {"0"}:
            try:
                before = git("merge-base", after, "origin/main")
                # First repository push has no prior release or usable base.
                full = before == after
            except subprocess.CalledProcessError:
                full = True
        if not full:
            paths = changed_paths(before, after)
    matrix = select(paths, full) if pipeline_mode in ("development", "prepare") else {"include": []}
    emit("mode", pipeline_mode)
    emit("matrix", matrix)
    emit("has-work", str(bool(matrix["include"])).lower())
    emit("version", "0.0.0-sha-"+after[:12] if pipeline_mode != "release" else ref.removeprefix("refs/tags/"))


if __name__ == "__main__":
    main()
