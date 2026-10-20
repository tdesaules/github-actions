#!/usr/bin/env python3
"""Prepare an isolated TLS endpoint reachable by actual native Docker actions."""
import argparse
import os
from pathlib import Path
import subprocess
import ssl
import time
import urllib.request

from ci import emit
from images import ENGINE, ROOT, action_ref, mount, run
from oci_archive import inspect_archive

REGISTRY = 'docker.io/library/registry:3.0.0@sha256:6c5666b861f3505b116bb9aa9b25175e71210414bd010d92035ff64018f9457e'
SKOPEO = 'ghcr.io/tdesaules/github-actions/oci-import-image@sha256:11f4439932710275ca5ffce92e71eef3680e09907c99c2eb0b83dfd32ebbdba1'


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('--signer-version')
    parser.add_argument('--verifier-version')
    args = parser.parse_args()
    directory = ROOT / '.build/fixtures'
    gateway = subprocess.check_output([ENGINE, 'network', 'inspect', 'bridge', '--format', '{{(index .IPAM.Config 0).Gateway}}'], text=True).strip()
    run('openssl', 'req', '-x509', '-newkey', 'rsa:2048', '-nodes', '-days', '1',
        '-keyout', directory/'registry-key.pem', '-out', directory/'registry-cert.pem',
        '-subj', '/CN=registry', '-addext', 'subjectAltName=IP:'+gateway,
        stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
    run(ENGINE, 'run', '-d', '--name', 'action-test-registry', '-p', '0:5000',
        '-v', mount(directory, '/certs', True),
        '-e', 'REGISTRY_HTTP_TLS_CERTIFICATE=/certs/registry-cert.pem',
        '-e', 'REGISTRY_HTTP_TLS_KEY=/certs/registry-key.pem', REGISTRY)
    # inspect avoids parsing Docker's additional IPv6 port mapping line.
    port = subprocess.check_output([ENGINE, 'inspect', 'action-test-registry', '--format', '{{(index (index .NetworkSettings.Ports "5000/tcp") 0).HostPort}}'], text=True).strip()
    repository = f'{gateway}:{port}/fixture'
    context = ssl.create_default_context(cafile=str(directory/'registry-cert.pem'))
    for attempt in range(30):
        try:
            with urllib.request.urlopen(f'https://{gateway}:{port}/v2/', context=context, timeout=2):
                break
        except OSError:
            if attempt == 29:
                raise
            time.sleep(0.2)
    digest = inspect_archive(directory/'fixture.oci.tar')['digest']
    emit('destination', 'docker://'+repository+'@'+digest)
    emit('reference', repository+'@'+digest)
    emit('repository', 'docker://'+repository)
    if args.signer_version or args.verifier_version:
        run(ENGINE, 'run', '--rm', '-v', mount(directory, '/fixtures'),
            '-e', 'SSL_CERT_FILE=/fixtures/registry-cert.pem', '--entrypoint=/usr/bin/skopeo',
            '--entrypoint=/usr/bin/skopeo', SKOPEO, 'copy', '--retry-times=3', '--preserve-digests',
            'oci-archive:/fixtures/fixture.oci.tar', 'docker://'+repository+'@'+digest)
        run(ENGINE, 'run', '--rm', '--user', f'{os.getuid()}:{os.getgid()}',
            '-v', mount(directory, '/fixtures'), '-e', 'COSIGN_PASSWORD=',
            '--entrypoint=/usr/bin/cosign', action_ref('new-image-signature' if args.signer_version else 'test-image-signature', args.signer_version or args.verifier_version),
            'generate-key-pair', '--output-key-prefix', '/fixtures/cosign')
        if args.verifier_version:
            # Isolated key fixture for the verifier's independent image job.
            cosign = action_ref('test-image-signature', args.verifier_version)
            run(ENGINE, 'run', '--rm', '--user', f'{os.getuid()}:{os.getgid()}', '-v', mount(directory, '/fixtures'),
                '--entrypoint=/usr/bin/cosign', cosign, 'signing-config', 'create', '--out', '/fixtures/signing.json')
            run(ENGINE, 'run', '--rm', '--user', f'{os.getuid()}:{os.getgid()}', '-v', mount(directory, '/fixtures'),
                '-e', 'SSL_CERT_FILE=/fixtures/registry-cert.pem', '-e', 'COSIGN_PASSWORD=',
                '--entrypoint=/usr/bin/cosign', cosign, 'sign', '--yes', '--key', '/fixtures/cosign.key',
                '--signing-config', '/fixtures/signing.json', repository+'@'+digest)


if __name__ == '__main__':
    main()
