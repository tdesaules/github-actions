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
import sys

from images import ROOT, ENGINE, architecture, container, local_ref, smoke, mount, run
from ci import discover
from reports import SYFT


def fixture_archive(path, arch="amd64"):
    """Small valid OCI archive, independent from the builder under test."""
    blobs = {}

    def blob(data):
        digest = hashlib.sha256(data).hexdigest()
        blobs["blobs/sha256/"+digest] = data
        return {"digest": "sha256:"+digest, "size": len(data)}

    layer_data = io.BytesIO()
    with tarfile.open(fileobj=layer_data, mode="w") as tar:
        temporary = tarfile.TarInfo('tmp')
        temporary.type = tarfile.DIRTYPE
        temporary.mode = 0o1777
        tar.addfile(temporary)
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
    fixture_archive(workspace / 'arm64.oci.tar', 'arm64')
    checksums = {name: hashlib.sha256((workspace / name).read_bytes()).hexdigest()
                 for name in ('fixture.oci.tar', 'arm64.oci.tar')}
    (workspace / 'fixture-checksums.json').write_text(json.dumps(checksums))
    (workspace / 'fixture-build.json').write_text(json.dumps({
        'image': 'fixture', 'arch': 'amd64', 'revision': '0' * 40,
        'version': 'fixture', 'archive_sha256': checksums['fixture.oci.tar'], 'tested': False}))
    (workspace / 'syft-empty.json').write_text(json.dumps({'artifacts': [], 'files': []}))
    (workspace / 'review-empty.json').write_text(json.dumps({'ignore': []}))


def negative(ref, operation, workspace, inputs, extra=None):
    try:
        container(ref, operation, workspace, inputs, extra=extra or [])
    except subprocess.CalledProcessError:
        return
    raise AssertionError(f"invalid {operation} input unexpectedly succeeded")


def test(image, version):
    smoke(image, version)
    if image.startswith('check-'):
        run(sys.executable, ROOT/'scripts/test-checks.py', '--image', image, '--version', version)
        return
    ref = local_ref(image, version)
    with tempfile.TemporaryDirectory(prefix="oci-action-tests-") as tmp:
        workspace = Path(tmp)
        prepare(workspace)
        before = (workspace / "fixture with spaces" / "hello.txt").read_bytes()
        if image == "oci-build-image":
            container(ref, "build-image", workspace, {"context": "fixture with spaces", "containerfile": "fixture with spaces/Containerfile", "image": "localhost/test", "version": "0.0.0-bootstrap", "platform": "linux/"+architecture(), "output": "result with spaces.oci.tar"}, extra=["--user=0"])
            assert tarfile.is_tarfile(workspace / "result with spaces.oci.tar")
            negative(ref, "build-image", workspace, {"image": "test", "version": "1.0.0", "platform": "invalid"})
        elif image == "oci-import-image":
            container(ref, "import-image", workspace, {"source": "oci-archive:fixture.oci.tar", "destination": "oci:layout with spaces:fixture"})
            negative(ref, "import-image", workspace, {"source": "oci-archive:missing.tar", "destination": "oci:invalid"})
        elif image == 'oci-export-image':
            container(ref, 'export-image', workspace, {'source': 'oci-archive:fixture.oci.tar', 'destination': 'oci-archive:result with spaces.oci.tar:fixture'})
            assert tarfile.is_tarfile(workspace / 'result with spaces.oci.tar')
        elif image == "oci-new-sbom":
            container(ref, "new-sbom", workspace, {"source": "dir:fixture with spaces", "output": "reports with spaces/sbom.json"},
                      extra=['--read-only', '--tmpfs', '/tmp:rw,mode=1777'])
            assert json.loads((workspace / "reports with spaces/sbom.json").read_text())["spdxVersion"].startswith("SPDX-")
            negative(ref, "new-sbom", workspace, {"source": "dir:missing", "output": "invalid.json"})
            workspace.chmod(0o555)
            try:
                negative(ref, 'new-sbom', workspace, {'source': 'dir:fixture with spaces', 'output': 'unwritable.json'},
                         extra=['--user=65532:65532'])
                assert not (workspace / 'unwritable.json').exists()
            finally:
                workspace.chmod(0o700)
        elif image == "oci-test-vulnerabilities":
            # An actual database download and scan, not a mocked successful report.
            run(ENGINE, 'run', '--rm', '-v', mount(workspace, '/fixtures'), SYFT,
                'scan', 'oci-archive:/fixtures/fixture.oci.tar', '-o', 'spdx-json=/fixtures/sbom.json')
            container(ref, "test-vulnerabilities", workspace, {"sbom": "sbom.json", "output": "reports with spaces/grype.json"})
            assert "matches" in json.loads((workspace / "reports with spaces/grype.json").read_text())
            workspace.chmod(0o1777)
            container(ref, 'test-vulnerabilities', workspace,
                      {'sbom': 'sbom.json', 'output': 'nonroot-grype.json'}, extra=['--user=65532:65532'])
            assert 'matches' in json.loads((workspace / 'nonroot-grype.json').read_text())
            negative(ref, "test-vulnerabilities", workspace, {"sbom": "missing.json", "fail-on": "invalid"})
        elif image == "oci-new-image-signature":
            container(ref, "new-image-signature", workspace, {"enabled": "false"})
            negative(ref, "new-image-signature", workspace, {"image": "example:latest"})
        elif image == 'oci-test-image-signature':
            negative(ref, "test-image-signature", workspace, {"image": "example@sha256:"+"0"*64})
        elif image == 'oci-merge-image-index':
            negative(ref, 'merge-image-index', workspace, {'sources': '[]'})
        elif image == 'oci-publish-image':
            negative(ref, 'publish-image', workspace, {'source': 'docker://example:latest', 'destination': 'docker://example:latest'})
        elif image == 'oci-get-image-digest':
            negative(ref, 'get-image-digest', workspace, {})
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
    for image in ([args.image] if args.image != "all" else sorted(discover()[1])):
        test(image, args.version)


if __name__ == "__main__":
    main()
