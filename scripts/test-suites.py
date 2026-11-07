#!/usr/bin/env python3
"""Exercise suite actions with read-only sources and genuine test failures."""
import argparse
from pathlib import Path
import subprocess
import tempfile

from images import ENGINE, ROOT, local_ref, mount


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
        elif image == 'check-test-pantavisor-system':
            package = good / 'tools/pvbuild'
            package.mkdir(parents=True)
            (good / 'tools/__init__.py').touch()
            (package / '__init__.py').touch()
            target = package / '__main__.py'
            target.write_text((ROOT / 'tests/fixtures/pantavisor-system/tools/pvbuild/__main__.py').read_text())
            failure = 'raise SystemExit("intentional failure")\n'
        elif image == 'check-test-catalog':
            component = good / 'check/test-fixture'
            component.mkdir(parents=True)
            (component / 'action.yml').write_text('runs:\n  image: docker://example.invalid/check-test-fixture@sha256:' + 'a' * 64 + '\n')
            (component / 'main.go').write_text('package main\n')
            target = component / 'Containerfile'
            target.write_text('FROM scratch AS final\n')
            failure = 'FROM alpine:latest AS final\n'
        else:
            raise ValueError(image)
        # Go executes compiled test binaries here; Docker defaults tmpfs to noexec.
        command = [ENGINE, 'run', '--rm', '--read-only', '--tmpfs', '/tmp:rw,exec,mode=1777',
                   '-v', mount(root, '/github/workspace', True),
                   '-e', 'INPUT_DIRECTORY=missing', reference, '--directory=project with spaces']
        subprocess.run(command, check=True)
        if image == 'check-test-python':
            # The published default remains tests/unit. Exercise the additional
            # candidate interface before changing the public action metadata.
            custom = good / 'tests with spaces'
            custom.mkdir()
            custom_test = custom / 'test_custom.py'
            custom_test.write_text('import unittest\nclass Fixture(unittest.TestCase):\n def test_fixture(self): self.assertTrue(True)\n')
            subprocess.run(command + ['--tests-directory=tests with spaces'], check=True)
            custom_test.write_text(failure)
            failed = subprocess.run(command + ['--tests-directory=tests with spaces'], capture_output=True, text=True)
            if failed.returncode == 0 or 'intentional failure' not in failed.stdout + failed.stderr:
                raise AssertionError('custom test directory did not propagate its test failure')
            for value in ('', '../outside', '/tmp', 'missing'):
                rejected = subprocess.run(command + ['--tests-directory=' + value], capture_output=True, text=True)
                if rejected.returncode == 0:
                    raise AssertionError(f'invalid test directory accepted: {value!r}')
            # Explicit CLI paths override an invalid environment value.
            configured = command.copy()
            configured[configured.index(reference):configured.index(reference)] = ['-e', 'INPUT_TESTS-DIRECTORY=missing']
            subprocess.run(configured + ['--tests-directory=tests/unit'], check=True)
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
