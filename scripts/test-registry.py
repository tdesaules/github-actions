#!/usr/bin/env python3
"""Integration test against an isolated local TLS registry, with no external push."""
import hashlib
import argparse
import json
from pathlib import Path
import ssl
import subprocess
import tempfile
import time
import urllib.request
import urllib.error
import uuid

from images import ENGINE, container, action_ref, mount, run
from oci_archive import inspect_archive
from testing import fixture_archive

REGISTRY = "docker.io/library/registry:3.0.0@sha256:6c5666b861f3505b116bb9aa9b25175e71210414bd010d92035ff64018f9457e"


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('--version', default='0.0.0-bootstrap')
    parser.add_argument('--index-only', action='store_true')
    args = parser.parse_args()
    name = "oci-test-"+uuid.uuid4().hex[:12]
    with tempfile.TemporaryDirectory(prefix="oci-registry-") as tmp:
        workspace = Path(tmp)
        run("openssl", "req", "-x509", "-newkey", "rsa:2048", "-nodes", "-days", "1", "-keyout", workspace / "key.pem", "-out", workspace / "cert.pem", "-subj", "/CN=registry", "-addext", "subjectAltName=DNS:registry,DNS:localhost,IP:127.0.0.1", stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
        run(ENGINE, "network", "create", name)
        try:
            run(ENGINE, "run", "-d", "--name", name, "--network", name, "--network-alias", "registry", "-p", "127.0.0.1::5000", "-v", mount(workspace, "/certs", True), "-e", "REGISTRY_HTTP_TLS_CERTIFICATE=/certs/cert.pem", "-e", "REGISTRY_HTTP_TLS_KEY=/certs/key.pem", REGISTRY)
            port = subprocess.check_output([ENGINE, "port", name, "5000"], text=True).strip().rsplit(":", 1)[1]
            context = ssl.create_default_context(cafile=str(workspace / "cert.pem"))
            for attempt in range(30):
                try:
                    with urllib.request.urlopen(f"https://localhost:{port}/v2/", context=context, timeout=2):
                        break
                except OSError:
                    if attempt == 29:
                        raise
                    time.sleep(0.2)
            extra = ["--network", name, "-e", "SSL_CERT_FILE=/github/workspace/cert.pem"]
            expected = {}
            for arch in ("amd64", "arm64"):
                archive = workspace / (arch+".tar")
                fixture_archive(archive, arch)
                expected[arch] = inspect_archive(archive)["digest"]
                if not args.index_only:
                    container(action_ref('publish-image', args.version), "publish-image", workspace, {"source": "oci-archive:"+archive.name, "destination": "docker://registry:5000/fixture@"+expected[arch]}, extra)
                    container(action_ref('get-image-digest', args.version), "get-image-digest", workspace, {"image": "registry:5000/fixture@"+expected[arch], "expected-digest": expected[arch]}, extra)
                else:
                    # Model an interrupted publication with children already present.
                    run(ENGINE, 'run', '--rm', '--network', name, '-v', mount(workspace, '/github/workspace'),
                        '-e', 'SSL_CERT_FILE=/github/workspace/cert.pem', '--entrypoint=/usr/bin/skopeo',
                        'quay.io/skopeo/stable:v1.22.3@sha256:7e735527d9d64c7456fc82d7f954c380b4a5dd1a8173115ce2ed7b3442ef36e0',
                        'copy', '--preserve-digests', 'oci-archive:/github/workspace/'+archive.name,
                        'docker://registry:5000/fixture@'+expected[arch])
            inputs = {"sources": '["amd64.tar","arm64.tar"]', "destination": "docker://registry:5000/fixture"}
            checksums = {arch: hashlib.sha256((workspace/(arch+'.tar')).read_bytes()).hexdigest() for arch in expected}
            # Fail after child publication by removing trust in the isolated registry.
            try:
                container(action_ref('merge-image-index', args.version), 'merge-image-index', workspace,
                          inputs, ['--network', name, '-e', 'SSL_CERT_FILE=/missing-ca.pem'])
            except subprocess.CalledProcessError:
                pass
            else:
                raise AssertionError('publication unexpectedly accepted an untrusted registry')
            if (workspace/'image-index.json').exists():
                raise AssertionError('failed publication emitted a success manifest')
            container(action_ref('merge-image-index', args.version), "merge-image-index", workspace, inputs, extra)
            index_digest = 'sha256:'+hashlib.sha256((workspace/'image-index.json').read_bytes()).hexdigest()
            # Retry again after success: both the manifest and tested archives must stay exact.
            container(action_ref('merge-image-index', args.version), 'merge-image-index', workspace, inputs, extra)
            if 'sha256:'+hashlib.sha256((workspace/'image-index.json').read_bytes()).hexdigest() != index_digest:
                raise AssertionError('successful retry changed index digest')
            if checksums != {arch: hashlib.sha256((workspace/(arch+'.tar')).read_bytes()).hexdigest() for arch in expected}:
                raise AssertionError('resume changed tested archives')
            request = urllib.request.Request(f"https://localhost:{port}/v2/fixture/manifests/{index_digest}", headers={"Accept": "application/vnd.oci.image.index.v1+json,application/vnd.docker.distribution.manifest.list.v2+json"})
            with urllib.request.urlopen(request, context=context) as response:
                index = json.load(response)
            actual = {m["platform"]["architecture"]: m["digest"] for m in index["manifests"]}
            if actual != expected:
                raise AssertionError(f"index changed child digests: {actual} != {expected}")
            try:
                with urllib.request.urlopen(f'https://localhost:{port}/v2/fixture/tags/list', context=context) as response:
                    if json.load(response).get('tags'):
                        raise AssertionError('digest-only publication unexpectedly created tags')
            except urllib.error.HTTPError as error:
                if error.code != 404 or not any(e['code'] == 'NAME_UNKNOWN' for e in json.load(error)['errors']):
                    raise
            if args.index_only:
                print('TLS index publication with unchanged two-platform fixture digests: PASS')
                return
            run(ENGINE, "run", "--rm", "--entrypoint=/usr/bin/cosign", "-v", mount(workspace, "/github/workspace"), "-e", "COSIGN_PASSWORD=", action_ref('new-image-signature', args.version), "generate-key-pair", "--output-key-prefix", "/github/workspace/cosign")
            reference = "registry:5000/fixture@"+expected["amd64"]
            key_extra = extra + ["-e", "COSIGN_PASSWORD="]
            container(action_ref('new-image-signature', args.version), "new-image-signature", workspace, {"image": reference, "key": "cosign.key", "transparency-log": "false"}, key_extra)
            container(action_ref('test-image-signature', args.version), "test-image-signature", workspace, {"image": reference, "key": "cosign.pub", "transparency-log": "false", "output": "signature.json"}, key_extra)
            if not json.loads((workspace / "signature.json").read_text()):
                raise AssertionError("empty signature verification report")
            print("TLS publication, remote digest, two-platform fixture index and real key-based signature/verification: PASS (not GitHub OIDC or ARM64 executable builds)")
        finally:
            subprocess.run([ENGINE, "rm", "-f", name], check=False, stdout=subprocess.DEVNULL)
            subprocess.run([ENGINE, "network", "rm", name], check=False, stdout=subprocess.DEVNULL)


if __name__ == "__main__":
    main()
