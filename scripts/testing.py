#!/usr/bin/env python3
"""Real-container integration checks. Never claims to emulate GitHub's runner."""
import argparse
import hashlib
import io
import json
from pathlib import Path
import subprocess
import tarfile
import tempfile

from images import ROOT, architecture, container, local_ref, smoke


def fixture_archive(path, arch="amd64"):
    """Small valid OCI archive, independent from the builder under test."""
    blobs = {}

    def blob(data):
        digest = hashlib.sha256(data).hexdigest()
        blobs["blobs/sha256/"+digest] = data
        return {"digest": "sha256:"+digest, "size": len(data)}

    layer_data = io.BytesIO()
    with tarfile.open(fileobj=layer_data, mode="w") as tar:
        info = tarfile.TarInfo("hello.txt")
        data = b"hello\n"
        info.size = len(data)
        tar.addfile(info, io.BytesIO(data))
    layer = blob(layer_data.getvalue())
    layer["mediaType"] = "application/vnd.oci.image.layer.v1.tar"
    config = blob(json.dumps({"architecture": arch, "os": "linux", "config": {}, "rootfs": {"type": "layers", "diff_ids": [layer["digest"]]}}).encode())
    config["mediaType"] = "application/vnd.oci.image.config.v1+json"
    manifest = blob(json.dumps({"schemaVersion": 2, "mediaType": "application/vnd.oci.image.manifest.v1+json", "config": config, "layers": [layer]}).encode())
    manifest["mediaType"] = "application/vnd.oci.image.manifest.v1+json"
    manifest["annotations"] = {"org.opencontainers.image.ref.name": "fixture"}
    blobs["index.json"] = json.dumps({"schemaVersion": 2, "mediaType": "application/vnd.oci.image.index.v1+json", "manifests": [manifest]}).encode()
    blobs["oci-layout"] = b'{"imageLayoutVersion":"1.0.0"}'
    with tarfile.open(path, "w") as tar:
        for name in ("blobs", "blobs/sha256"):
            info = tarfile.TarInfo(name)
            info.type = tarfile.DIRTYPE
            info.mode = 0o755
            tar.addfile(info)
        for name, data in blobs.items():
            info = tarfile.TarInfo(name)
            info.size = len(data)
            tar.addfile(info, io.BytesIO(data))


def prepare(workspace):
    directory = workspace / "fixture with spaces"
    directory.mkdir(parents=True, exist_ok=True)
    (directory / "hello.txt").write_text("hello\n")
    (directory / "Containerfile").write_text('FROM scratch\nCOPY hello.txt /hello.txt\n')
    fixture_archive(workspace / "fixture.oci.tar")


def negative(ref, operation, workspace, inputs):
    try:
        container(ref, operation, workspace, inputs)
    except subprocess.CalledProcessError:
        return
    raise AssertionError(f"invalid {operation} input unexpectedly succeeded")


def test(image, version):
    smoke(image, version)
    ref = local_ref(image, version)
    with tempfile.TemporaryDirectory(prefix="oci-action-tests-") as tmp:
        workspace = Path(tmp)
        prepare(workspace)
        before = (workspace / "fixture with spaces" / "hello.txt").read_bytes()
        if image == "buildah":
            container(ref, "build-image", workspace, {"context": "fixture with spaces", "containerfile": "fixture with spaces/Containerfile", "image": "localhost/test", "version": "0.0.0-bootstrap", "platform": "linux/"+architecture(), "output": "result with spaces.oci.tar"}, extra=["--user=0"])
            assert tarfile.is_tarfile(workspace / "result with spaces.oci.tar")
            negative(ref, "build-image", workspace, {"image": "test", "version": "1.0.0", "platform": "invalid"})
        elif image == "skopeo":
            container(ref, "import-image", workspace, {"source": "oci-archive:fixture.oci.tar", "destination": "oci:layout with spaces:fixture"})
            container(ref, "export-image", workspace, {"source": "oci:layout with spaces:fixture", "destination": "oci-archive:result with spaces.oci.tar:fixture"})
            assert tarfile.is_tarfile(workspace / "result with spaces.oci.tar")
            negative(ref, "import-image", workspace, {"source": "oci-archive:missing.tar", "destination": "oci:invalid"})
        elif image == "syft":
            container(ref, "new-sbom", workspace, {"source": "dir:fixture with spaces", "output": "reports with spaces/sbom.json"})
            assert json.loads((workspace / "reports with spaces/sbom.json").read_text())["spdxVersion"].startswith("SPDX-")
            negative(ref, "new-sbom", workspace, {"source": "dir:missing", "output": "invalid.json"})
        elif image == "grype":
            # An actual database download and scan, not a mocked successful report.
            container(local_ref("syft", version), "new-sbom", workspace, {"source": "oci-archive:fixture.oci.tar", "output": "sbom.json"})
            container(ref, "test-vulnerabilities", workspace, {"sbom": "sbom.json", "output": "reports with spaces/grype.json"})
            assert "matches" in json.loads((workspace / "reports with spaces/grype.json").read_text())
            negative(ref, "test-vulnerabilities", workspace, {"sbom": "missing.json", "fail-on": "invalid"})
        elif image == "cosign":
            container(ref, "new-image-signature", workspace, {"enabled": "false"})
            negative(ref, "new-image-signature", workspace, {"image": "example:latest"})
            negative(ref, "test-image-signature", workspace, {"image": "example@sha256:"+"0"*64})
        assert (workspace / "fixture with spaces" / "hello.txt").read_bytes() == before


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--image", default="all")
    parser.add_argument("--version", default="0.0.0-bootstrap")
    parser.add_argument("--prepare", type=Path)
    args = parser.parse_args()
    if args.prepare:
        prepare(args.prepare)
        return
    for image in ([args.image] if args.image != "all" else ["buildah", "skopeo", "syft", "grype", "cosign"]):
        test(image, args.version)


if __name__ == "__main__":
    main()
