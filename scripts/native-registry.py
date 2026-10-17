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
from images import ENGINE, ROOT, local_ref, mount, run
from oci_archive import inspect_archive

REGISTRY = 'docker.io/library/registry:3.0.0@sha256:6c5666b861f3505b116bb9aa9b25175e71210414bd010d92035ff64018f9457e'
SKOPEO = 'quay.io/skopeo/stable:v1.22.3@sha256:7e735527d9d64c7456fc82d7f954c380b4a5dd1a8173115ce2ed7b3442ef36e0'


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('--signer-version')
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
    emit('destination', 'docker://'+repository+':tested')
    emit('reference', repository+'@'+digest)
    if args.signer_version:
        run(ENGINE, 'run', '--rm', '-v', mount(directory, '/fixtures'),
            '-e', 'SSL_CERT_FILE=/fixtures/registry-cert.pem', '--entrypoint=/usr/bin/skopeo',
            SKOPEO, 'copy', '--retry-times=3', '--preserve-digests',
            'oci-archive:/fixtures/fixture.oci.tar', 'docker://'+repository+':tested')
        run(ENGINE, 'run', '--rm', '--user', f'{os.getuid()}:{os.getgid()}',
            '-v', mount(directory, '/fixtures'), '-e', 'COSIGN_PASSWORD=',
            '--entrypoint=/usr/bin/cosign', local_ref('cosign', args.signer_version),
            'generate-key-pair', '--output-key-prefix', '/fixtures/cosign')


if __name__ == '__main__':
    main()
