#!/usr/bin/env python3
"""Exercise curl TLS backends against an isolated HTTPS fixture."""
import argparse
from functools import partial
import http.server
import json
import os
from pathlib import Path
import ssl
import subprocess
import tempfile
import threading

parser = argparse.ArgumentParser(description=__doc__)
parser.add_argument('--reference', required=True)
parser.add_argument('--platform', default='linux/amd64')
parser.add_argument('--curl-version', default='8.23.0')
parser.add_argument('--http3', action=argparse.BooleanOptionalAction, default=True,
                    help='require HTTP/3 (enabled for the historical curl backport)')
args = parser.parse_args()


def run(command, **kwargs):
    return subprocess.run(command, check=True, text=True, capture_output=True, **kwargs).stdout.strip()


class Handler(http.server.SimpleHTTPRequestHandler):
    def handle(self):
        try:
            super().handle()
        except (BrokenPipeError, ConnectionResetError):
            # Negative certificate tests deliberately disconnect before HTTP.
            pass

    def log_message(self, *_args):
        pass

    def do_GET(self):
        if self.path == '/redirect':
            self.send_response(302)
            self.send_header('Location', '/payload')
            self.end_headers()
        else:
            super().do_GET()


with tempfile.TemporaryDirectory(prefix='kas-curl-') as directory:
    root = Path(directory)
    public = root / 'public'
    public.mkdir()
    (public / 'payload').write_text('verified HTTPS payload\n')
    run(['openssl', 'req', '-x509', '-newkey', 'rsa:2048', '-nodes', '-days', '1',
         '-subj', '/CN=localhost', '-addext', 'subjectAltName=DNS:localhost',
         '-keyout', str(root / 'key.pem'), '-out', str(public / 'ca.pem')])
    source = root / 'source'
    run(['git', 'init', '-b', 'main', str(source)])
    (source / 'payload').write_text('verified Git payload\n')
    env = os.environ | {'GIT_CONFIG_GLOBAL': '/dev/null', 'GIT_CONFIG_NOSYSTEM': '1',
                       'GIT_CONFIG_COUNT': '0', 'GIT_AUTHOR_NAME': 'Fixture',
                       'GIT_AUTHOR_EMAIL': 'fixture@example.invalid',
                       'GIT_COMMITTER_NAME': 'Fixture', 'GIT_COMMITTER_EMAIL': 'fixture@example.invalid'}
    run(['git', 'add', 'payload'], cwd=source, env=env)
    run(['git', 'commit', '-m', 'test: HTTPS fixture'], cwd=source, env=env)
    revision = run(['git', 'rev-parse', 'HEAD'], cwd=source, env=env)
    bare = public / 'repository.git'
    run(['git', 'clone', '--bare', str(source), str(bare)], env=env)
    run(['git', 'update-server-info'], cwd=bare, env=env)
    server = http.server.ThreadingHTTPServer(('127.0.0.1', 0), partial(Handler, directory=str(public)))
    context = ssl.SSLContext(ssl.PROTOCOL_TLS_SERVER)
    context.load_cert_chain(public / 'ca.pem', root / 'key.pem')
    server.socket = context.wrap_socket(server.socket, server_side=True)
    thread = threading.Thread(target=server.serve_forever, daemon=True)
    thread.start()
    port = server.server_address[1]
    (public / 'kas.yml').write_text(json.dumps({
        'header': {'version': 16},
        'repos': {'fixture': {'url': f'https://localhost:{port}/repository.git',
                              'commit': revision, 'path': 'repo'}},
    }))
    base = ['podman', 'run', '--rm', '--platform', args.platform, '--network=host',
            '--security-opt', 'label=disable', '-v', f'{public}:/fixtures:ro']

    def container(entrypoint, *command, check=True):
        result = subprocess.run(base + ['--entrypoint', entrypoint, args.reference, *command],
                                text=True, capture_output=True)
        if check and result.returncode:
            raise RuntimeError(result.stdout + result.stderr)
        return result

    try:
        container('/usr/bin/python3', '-c',
                  'import ctypes, sys\n'
                  'for name, backend in [("libcurl.so.4", b"OpenSSL"), ("libcurl-gnutls.so.4", b"GnuTLS")]:\n'
                  ' lib = ctypes.CDLL(name)\n'
                  ' lib.curl_version.restype = ctypes.c_char_p\n'
                  ' version = lib.curl_version()\n'
                  ' assert ("libcurl/" + sys.argv[1]).encode() in version and backend in version, version\n',
                  args.curl_version)
        version = container('/usr/bin/curl', '--version').stdout
        assert f'curl {args.curl_version} ' in version and f'libcurl/{args.curl_version} ' in version, version
        features = ('OpenSSL/', 'HTTP2', 'GSS-API', 'IDN', 'PSL')
        if args.http3:
            features += ('HTTP3',)
        for feature in features:
            assert feature in version, (feature, version)
        result = container('/usr/bin/curl', '--fail', '--location', '--noproxy', '*',
                           '--cacert', '/fixtures/ca.pem', f'https://localhost:{port}/redirect')
        assert result.stdout == 'verified HTTPS payload\n', result.stdout
        for host, trust in [('localhost', []), ('127.0.0.1', ['--cacert', '/fixtures/ca.pem'])]:
            result = container('/usr/bin/curl', '--noproxy', '*', *trust,
                               f'https://{host}:{port}/payload', check=False)
            assert result.returncode == 60, (host, result.returncode, result.stderr)
        # This exercises Git's existing CURL_GNUTLS_3 ABI, not just dlopen.
        clone = container('/bin/sh', '-ec',
                          'git -c http.sslCAInfo=/fixtures/ca.pem clone "$1" /tmp/clone >&2; '
                          'git -C /tmp/clone rev-parse HEAD; cat /tmp/clone/payload',
                          'fixture', f'https://localhost:{port}/repository.git')
        assert clone.stdout == revision + '\nverified Git payload\n', clone.stdout
        rejected = container('/usr/bin/git', 'ls-remote',
                             f'https://localhost:{port}/repository.git', check=False)
        assert rejected.returncode != 0 and 'certificate' in rejected.stderr.lower(), rejected.stderr
        checkout = container('/bin/sh', '-ec',
                             'cat /fixtures/ca.pem >> /etc/ssl/certs/ca-certificates.crt; '
                             'mkdir /tmp/kas; export KAS_WORK_DIR=/tmp/kas '
                             'KAS_BUILD_DIR=/tmp/kas-build; '
                             'kas checkout --skip setup_environ --skip write_bbconfig /fixtures/kas.yml >&2; '
                             'git -C /tmp/kas/repo rev-parse HEAD')
        assert checkout.stdout.strip() == revision, checkout.stdout
        print(f'PASS: curl {args.curl_version}, HTTPS redirect, CA/hostname rejection, Git GnuTLS clone, CA rejection and KAS checkout; HTTP/3 required={args.http3}')
    finally:
        server.shutdown()
        server.server_close()
        thread.join()
