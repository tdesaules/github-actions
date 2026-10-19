#!/usr/bin/env python3
"""Exercise suite actions with read-only sources and genuine test failures."""
import argparse
from pathlib import Path
import subprocess
import tempfile

from images import ENGINE, local_ref, mount


def test(image, reference):
    with tempfile.TemporaryDirectory(prefix='suite-fixture-') as temporary:
        root = Path(temporary)
        root.chmod(0o755)
        good = root / 'project with spaces'
        good.mkdir()
        if image == 'check-test-go':
            (good / 'go.mod').write_text('module example.invalid/fixture\n\ngo 1.27\n')
            target = good / 'fixture_test.go'
            target.write_text('package fixture\nimport "testing"\nfunc TestFixture(t *testing.T) {}\n')
            failure = 'package fixture\nimport "testing"\nfunc TestFixture(t *testing.T) { t.Fatal("intentional failure") }\n'
        elif image == 'check-test-python':
            (good / 'tests/unit').mkdir(parents=True)
            target = good / 'tests/unit/test_fixture.py'
            target.write_text('import unittest\nclass Fixture(unittest.TestCase):\n def test_fixture(self): self.assertTrue(True)\n')
            failure = 'import unittest\nclass Fixture(unittest.TestCase):\n def test_fixture(self): self.fail("intentional failure")\n'
        elif image == 'check-test-catalog':
            component = good / 'check/test-fixture'
            component.mkdir(parents=True)
            (component / 'action.yml').write_text('runs:\n  image: docker://example.invalid/check-test-fixture:unpublished\n')
            (component / 'main.go').write_text('package main\n')
            target = component / 'Containerfile'
            target.write_text('FROM scratch AS final\n')
            failure = 'FROM alpine:latest AS final\n'
        else:
            raise ValueError(image)
        command = [ENGINE, 'run', '--rm', '--read-only', '--tmpfs', '/tmp:rw,mode=1777',
                   '-v', mount(root, '/github/workspace', True),
                   '-e', 'INPUT_DIRECTORY=missing', reference, '--directory=project with spaces']
        subprocess.run(command, check=True)
        target.write_text(failure)
        result = subprocess.run(command, capture_output=True, text=True)
        expected = 'unpinned base' if image == 'check-test-catalog' else 'intentional failure'
        if result.returncode == 0 or expected not in result.stdout + result.stderr:
            raise AssertionError(f'expected real test failure: {result.stdout}\n{result.stderr}')
        print(f'{image}: read-only success, CLI precedence and negative test passed')


if __name__ == '__main__':
    parser = argparse.ArgumentParser()
    parser.add_argument('--image', required=True)
    parser.add_argument('--version', default='0.0.0-bootstrap')
    parser.add_argument('--reference')
    args = parser.parse_args()
    test(args.image, args.reference or local_ref(args.image, args.version))
