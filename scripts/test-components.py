#!/usr/bin/env python3
"""Test component verification without producer checkouts or registry access."""
import argparse
import hashlib
from pathlib import Path
import shutil
import subprocess
import tempfile

from images import ENGINE, ROOT, local_ref, mount


def test(reference):
    with tempfile.TemporaryDirectory(prefix='component-action-') as temporary:
        workspace = Path(temporary)
        workspace.chmod(0o755)
        component = workspace / 'component with spaces'
        shutil.copytree(ROOT / 'tests/fixtures/component', component)
        output = workspace / 'outputs'
        output.touch(mode=0o644)
        command = [ENGINE, 'run', '--rm', '--read-only',
                   '-v', mount(component, '/component', True),
                   '-v', mount(output, '/outputs'), '-e', 'GITHUB_OUTPUT=/outputs',
                   reference, '--directory=/component']
        subprocess.run(command, check=True)
        expected = hashlib.sha256((component / 'component.json').read_bytes()).hexdigest()
        if 'digest=sha256:' + expected not in output.read_text():
            raise AssertionError('descriptor digest output differs')
        descriptor = (component / 'component.json').read_bytes()
        for filename in ('rootfs.squashfs', 'runtime.json', 'build-provenance.json'):
            payload = component / filename
            original = payload.read_bytes()
            payload.write_bytes(original + b'tampered')
            output.write_text('')
            result = subprocess.run(command, capture_output=True, text=True)
            if result.returncode == 0 or output.read_text():
                raise AssertionError(f'accepted corrupted {filename}')
            payload.write_bytes(original)
        target = component / 'packages.manifest'
        target.unlink()
        target.symlink_to('rootfs.squashfs')
        result = subprocess.run(command, capture_output=True, text=True)
        if result.returncode == 0:
            raise AssertionError('accepted a symlink in the export')
        if (component / 'component.json').read_bytes() != descriptor:
            raise AssertionError('verification modified the descriptor')
        print('PASS: component output digest, corruption, symlink rejection and read-only inputs')


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--version', default='0.0.0-bootstrap')
    parser.add_argument('--reference')
    args = parser.parse_args()
    test(args.reference or local_ref('oci-test-component', args.version))
