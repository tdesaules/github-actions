#!/usr/bin/env python3
"""Publish, retry, fetch and sign a fixture in a private TLS registry only."""
import argparse
import hashlib
import json
from pathlib import Path
import shutil
import ssl
import subprocess
import tempfile
import time
import urllib.request
import uuid

from images import ENGINE, ROOT, mount

REGISTRY = 'docker.io/library/registry:3.0.0@sha256:6c5666b861f3505b116bb9aa9b25175e71210414bd010d92035ff64018f9457e'
SIGNER = 'ghcr.io/tdesaules/github-actions/oci-new-image-signature@sha256:af6ba568aea201f3b12d2946cd19fc72745e66977cc29f6b40c8df9486d31f02'
VERIFIER = 'ghcr.io/tdesaules/github-actions/oci-test-image-signature@sha256:1143280c113f541d0a8d1b160702dc9705266a3d3c9b8c4406ac4d33f1412070'


def main(version):
    name = 'component-test-' + uuid.uuid4().hex[:12]
    with tempfile.TemporaryDirectory(prefix='component-registry-') as temporary:
        root = Path(temporary)
        root.chmod(0o755)
        source = root / 'component'
        shutil.copytree(ROOT / 'tests/fixtures/component', source)
        metadata = json.loads((source / 'component.json').read_text())
        metadata['source']['dirty'] = False
        (source / 'build-provenance.json').write_text(json.dumps({'schema': 1, 'source': metadata['source']}))
        for filename in metadata['files']:
            data = (source / filename).read_bytes()
            metadata['files'][filename] = {'size': len(data), 'sha256': hashlib.sha256(data).hexdigest()}
        (source / 'component.json').write_text(json.dumps(metadata, sort_keys=True))
        subprocess.run(['openssl', 'req', '-x509', '-newkey', 'rsa:2048', '-nodes', '-days', '1',
                        '-keyout', str(root / 'key.pem'), '-out', str(root / 'cert.pem'), '-subj', '/CN=registry',
                        '-addext', 'subjectAltName=DNS:registry,DNS:localhost,IP:127.0.0.1'],
                       check=True, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
        subprocess.run([ENGINE, 'network', 'create', name], check=True, stdout=subprocess.DEVNULL)
        try:
            subprocess.run([ENGINE, 'run', '-d', '--name', name, '--network', name, '--network-alias', 'registry',
                            '-p', '127.0.0.1::5000', '-v', mount(root, '/certs', True),
                            '-e', 'REGISTRY_HTTP_TLS_CERTIFICATE=/certs/cert.pem',
                            '-e', 'REGISTRY_HTTP_TLS_KEY=/certs/key.pem', REGISTRY], check=True, stdout=subprocess.DEVNULL)
            port = subprocess.check_output([ENGINE, 'port', name, '5000'], text=True).strip().rsplit(':', 1)[1]
            trust = ssl.create_default_context(cafile=str(root / 'cert.pem'))
            for attempt in range(30):
                try:
                    with urllib.request.urlopen(f'https://localhost:{port}/v2/', context=trust, timeout=2):
                        break
                except OSError:
                    if attempt == 29:
                        raise
                    time.sleep(0.2)

            def action(image, args, *, ca='/github/workspace/cert.pem', fail=False):
                output = root / 'outputs'
                output.write_text('')
                command = [ENGINE, 'run', '--rm', '--network', name, '-v', mount(root, '/github/workspace'),
                           '-w', '/github/workspace', '-e', 'SSL_CERT_FILE=' + ca,
                           '-e', 'GITHUB_OUTPUT=/github/workspace/outputs', '-e', 'COSIGN_PASSWORD=', image, *args]
                result = subprocess.run(command, capture_output=True, text=True)
                if fail:
                    if result.returncode == 0:
                        raise AssertionError('invalid registry operation succeeded')
                    return {}
                if result.returncode:
                    raise AssertionError(result.stdout + result.stderr)
                return dict(line.split('=', 1) for line in output.read_text().splitlines())

            publisher = f'localhost/github-actions/oci-publish-component:{version}'
            fetcher = f'localhost/github-actions/oci-get-component:{version}'
            arguments = ['--directory=component', '--registry=registry:5000/fixture']
            action(publisher, arguments, ca='/missing-ca.pem', fail=True)
            first = action(publisher, arguments)
            second = action(publisher, arguments)
            if first != second:
                raise AssertionError('retry changed the published identity')
            action(fetcher, ['--reference=' + first['reference'], '--output=download'])
            for filename in [*metadata['files'], 'component.json']:
                if (source / filename).read_bytes() != (root / 'download' / filename).read_bytes():
                    raise AssertionError('roundtrip changed ' + filename)
            subprocess.run([ENGINE, 'run', '--rm', '-v', mount(root, '/github/workspace'),
                            '-e', 'COSIGN_PASSWORD=', '--entrypoint=/usr/bin/cosign', SIGNER,
                            'generate-key-pair', '--output-key-prefix', '/github/workspace/cosign'], check=True)
            action(SIGNER, ['--image=' + first['reference'], '--key=cosign.key', '--transparency-log=false'])
            action(VERIFIER, ['--image=' + first['reference'], '--key=cosign.pub', '--transparency-log=false', '--output=signature.json'])
            if not json.loads((root / 'signature.json').read_text()):
                raise AssertionError('signature verification produced no evidence')
            print('PASS: TLS component publication, identical retry, exact download, ORAS artifact signature and verification')
        finally:
            subprocess.run([ENGINE, 'rm', '-f', name], check=False, stdout=subprocess.DEVNULL)
            subprocess.run([ENGINE, 'network', 'rm', name], check=False, stdout=subprocess.DEVNULL)


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--version', default='pantavisor-candidate')
    main(parser.parse_args().version)
