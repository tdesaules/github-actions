#!/usr/bin/env python3
"""Qualify the selector image with literal inputs and runner output permissions."""
import argparse
from pathlib import Path
import subprocess
import tempfile

from images import ENGINE, local_ref, mount


def test(reference):
    with tempfile.TemporaryDirectory(prefix='pantavisor-selector-') as temporary:
        workspace = Path(temporary)
        workspace.chmod(0o755)
        output = workspace / 'step output'
        output.touch(mode=0o644)
        base = [ENGINE, 'run', '--rm', '--read-only',
                '-v', mount(workspace, '/github/workspace'),
                '-e', 'GITHUB_OUTPUT=/github/workspace/step output',
                '-e', 'INPUT_PROJECT=invalid', reference]
        for paths, full, expected in [
            ('[]', 'false', (False, False, False)),
            ('["README.md"]', 'false', (False, False, False)),
            ('["tests/test_package.py"]', 'false', (True, False, False)),
            ('["services/deleted file"]', 'false', (True, True, True)),
            ('["README.md"]', 'true', (True, True, True)),
        ]:
            output.write_text('')
            subprocess.run(base + ['--project=system', '--files=' + paths, '--full=' + full], check=True)
            got = dict(line.split('=', 1) for line in output.read_text().splitlines())
            want = dict(zip(('check-required', 'build-required', 'test-required'),
                            (str(value).lower() for value in expected)))
            if got != want:
                raise AssertionError((got, want))
        for args in [[], ['--project=unknown'], ['--project=core', '--files=null'],
                     ['--project=core', '--files={}'], ['--project=core', '--files=["../escape"]'],
                     ['--project=core', '--files=["a\\noutput=false"]'], ['--project=core', '--full=invalid']]:
            output.write_text('')
            result = subprocess.run(base + args, capture_output=True, text=True)
            if result.returncode == 0 or output.read_text():
                raise AssertionError(f'invalid input succeeded or wrote partial outputs: {args}')
        output.chmod(0o444)
        denied = base.copy()
        denied[2:2] = ['--user=65532:65532']
        result = subprocess.run(denied + ['--project=core'], capture_output=True, text=True)
        if result.returncode == 0 or 'permission denied' not in result.stderr:
            raise AssertionError('unwritable GITHUB_OUTPUT was not rejected')
        if sorted(p.name for p in workspace.iterdir()) != ['step output']:
            raise AssertionError('selector created unexpected workspace files')
        print('PASS: selector decisions, CLI precedence, malformed inputs, read-only rootfs and output permissions')


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--version', default='0.0.0-bootstrap')
    parser.add_argument('--reference')
    args = parser.parse_args()
    test(args.reference or local_ref('workflow-get-pantavisor-targets', args.version))
