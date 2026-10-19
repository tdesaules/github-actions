#!/usr/bin/env python3
"""Local/CI orchestration. Runtime actions do not require Python or Task."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import platform
import shutil
import subprocess
import sys
import tarfile
import tempfile

from ci import ROOT, discover, IMAGE
from oci_archive import inspect_archive

ENGINE = os.environ.get("CONTAINER_ENGINE", "podman")
if Path("/tmp/opencode").is_dir():
    tempfile.tempdir = "/tmp/opencode"
BOOTSTRAP = "quay.io/buildah/stable:v1.43.4@sha256:07e637ad7a1df37f44afea8287935b350224ac193d63e5cc96debb50a366853c"
SKOPEO_BOOTSTRAP = "quay.io/skopeo/stable:v1.22.3@sha256:6ca79dc9f56b60a0ca92a7b7ebc0cd0d1d9f13b70561fad07920d3d1dfade1c0"


def run(*args, **kwargs):
    print("+", " ".join(map(str, args)), flush=True)
    return subprocess.run(list(map(str, args)), check=True, **kwargs)


def architecture():
    return {"x86_64": "amd64", "aarch64": "arm64"}[platform.machine()]


def builder_options():
    # Qualified on hosted Ubuntu AMD64/ARM64; scope only to Buildah containers.
    return ['--security-opt=seccomp=unconfined', '--security-opt=apparmor=unconfined'] if ENGINE == 'docker' else []


def local_ref(image, version):
    return f"localhost/github-actions/{image}:{version}"


def action_ref(operation, version):
    return local_ref('oci-'+operation, version)


def load_archive(archive, reference):
    # OCI archives need not preserve a local tag. Address the loaded config ID.
    image_id = inspect_archive(archive)["image_id"]
    if ENGINE == "docker":
        # Docker's classic image store does not universally support OCI load.
        # Convert only the ephemeral execution transport, NEVER the publication
        # archive. The unchanged config ID identifies the tested rootfs/config.
        with tempfile.TemporaryDirectory(prefix="docker-load-") as tmp:
            run(ENGINE, "run", "--rm", "-v", mount(Path(archive).resolve().parent, "/input", True),
                "-v", mount(tmp, "/output"), SKOPEO_BOOTSTRAP, "copy",
                "oci-archive:/input/"+Path(archive).name, "docker-archive:/output/image.tar:"+reference)
            run(ENGINE, "load", "-i", Path(tmp) / "image.tar")
    else:
        run(ENGINE, "load", "-i", archive)
    run(ENGINE, "tag", image_id, reference)


def mount(path, target, readonly=False):
    # Private temporary copies prevent relabelling the user's repository on SELinux.
    suffix = ":Z" if ENGINE == "podman" else ""
    if readonly:
        suffix += ",ro" if suffix else ":ro"
    return f"{path}:{target}{suffix}"


def container(ref, operation, workspace, inputs=None, extra=None):
    cmd = [ENGINE, "run", "--rm", "-v", mount(workspace, "/github/workspace"), "-w", "/github/workspace"]
    if operation == 'build-image':
        cmd += builder_options()
    # Root in rootless Podman maps to the invoking host user. No privileged mode.
    env = os.environ.copy()
    for key, value in (inputs or {}).items():
        name = "INPUT_"+key.upper()
        env[name] = value
        cmd += ["-e", name]
    cmd += list(extra or []) + [ref]
    return run(*cmd, env=env)


def build(image, version, direct=False):
    _, owners = discover()
    ref = local_ref(image, version)
    out = ROOT / ".build" / image / architecture()
    out.mkdir(parents=True, exist_ok=True)
    revision = subprocess.check_output(["git", "rev-parse", "HEAD"], cwd=ROOT, text=True).strip()
    if direct:
        # Diagnostic path only. Does NOT validate nested Buildah or a Docker action.
        run(ENGINE, "build", "--platform", "linux/"+architecture(), "-f", owners[image]+"/Containerfile", "-t", ref, ".", cwd=ROOT)
        if ENGINE != "podman":
            raise ValueError("--direct archive export currently requires Podman; Docker save does not produce an OCI archive")
        run(ENGINE, "save", "--format=oci-archive", "-o", out / "image.oci.tar", ref)
        return
    with tempfile.TemporaryDirectory(prefix="oci-bootstrap-") as tmp:
        workspace = Path(tmp)
        shutil.copytree(ROOT, workspace / "source", ignore=shutil.ignore_patterns(".git", ".build", ".candidate-actions", "__pycache__"))
        if image == "oci-build-image":
            command = [ENGINE, "run", "--rm", *builder_options(), "--user=0", "-v", mount(workspace, "/workspace"), "-w", "/workspace/source", "--entrypoint=/bin/sh", BOOTSTRAP,
                       "-ec", 'buildah --storage-driver=vfs bud --isolation=chroot --network=host --format=oci --label "org.opencontainers.image.revision=$4" -f "$1" -t "$2" .; buildah --storage-driver=vfs push "$2" oci-archive:/workspace/image.oci.tar',
                       "bootstrap", owners[image]+"/Containerfile", ref, version, revision]
            run(*command)
        else:
            container(action_ref('build-image', version), "build-image", workspace,
                      {"context": "source", "containerfile": "source/"+owners[image]+"/Containerfile", "image": "localhost/github-actions/"+image, "version": version, "platform": "linux/"+architecture(), "output": "image.oci.tar", "labels": json.dumps({"org.opencontainers.image.revision": revision})}, extra=["--user=0"])
        shutil.copy2(workspace / "image.oci.tar", out / "image.oci.tar")
    load_archive(out / "image.oci.tar", ref)


def smoke(image, version):
    ref = local_ref(image, version)
    run(ENGINE, "run", "--rm", ref, "--tool-version")
    data = subprocess.check_output([ENGINE, "image", "inspect", ref])
    config = json.loads(data)[0]
    if config["Architecture"] != architecture():
        raise ValueError("image architecture mismatch")
    if config["Config"]["Entrypoint"] != ["/entrypoint"]:
        raise ValueError("unexpected entrypoint")


def candidates(image, version):
    actions, _ = discover()
    for directory, tool in actions.items():
        if image != "all" and tool != image:
            continue
        target = ROOT / ".candidate-actions" / Path(directory).name
        target.mkdir(parents=True, exist_ok=True)
        text = (ROOT / directory / "action.yml").read_text()
        # Native Docker actions pull docker:// references even if already loaded.
        # A temporary Dockerfile uses the locally loaded candidate instead.
        if tool == 'oci-build-image':
            text = IMAGE.sub('        BUILDER_IMAGE: docker://'+local_ref(tool, version), text)
        else:
            text = IMAGE.sub("  image: Dockerfile", text)
        (target / "action.yml").write_text(text)
        (target / "Dockerfile").write_text("FROM "+local_ref(tool, version)+"\n")


def metadata(image, version):
    from oci_archive import check_temporary_directory
    directory = ROOT / ".build" / image / architecture()
    archive = directory / "image.oci.tar"
    check_temporary_directory(archive)
    with archive.open("rb") as stream:
        checksum = hashlib.file_digest(stream, "sha256").hexdigest()
    data = {"image": image, "arch": architecture(), "version": version,
            "revision": subprocess.check_output(['git', 'rev-parse', 'HEAD'], cwd=ROOT, text=True).strip(),
            "archive_sha256": checksum,
            "tested": True}
    (directory / "metadata.json").write_text(json.dumps(data, indent=2)+"\n")


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("command", choices=("build-image", "test-image", "initialize-bootstrap", "new-candidate-actions", "new-metadata"))
    parser.add_argument("--image", default="all")
    parser.add_argument("--version", default="0.0.0-bootstrap")
    parser.add_argument("--direct", action="store_true", help="diagnostic engine build, not a nested Buildah qualification")
    args = parser.parse_args()
    _, owners = discover()
    images = [args.image] if args.image != "all" else ["oci-build-image"]+sorted(set(owners)-{"oci-build-image"})
    if args.command == "new-candidate-actions":
        candidates(args.image, args.version)
        return
    for image in images:
        if args.command in ("build-image", "initialize-bootstrap"):
            build(image, args.version, args.direct)
        if args.command in ("test-image", "initialize-bootstrap"):
            smoke(image, args.version)
        if args.command == "new-metadata":
            metadata(image, args.version)


if __name__ == "__main__":
    main()
