"""Real Git remotes exercise failed atomic pushes, retry and a competing writer."""
import importlib.util
import json
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest
from unittest.mock import patch

sys.path.insert(0, str(Path(__file__).resolve().parents[2] / 'scripts'))
SPEC = importlib.util.spec_from_file_location('recovery_release', Path(__file__).resolve().parents[2] / 'scripts/release.py')
release = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(release)


class RecoveryTests(unittest.TestCase):
    def setUp(self):
        self.temporary = tempfile.TemporaryDirectory(prefix='release recovery ')
        self.addCleanup(self.temporary.cleanup)
        self.root = Path(self.temporary.name)
        self.remote = self.root / 'remote.git'
        self.checkout = self.root / 'first'
        self.artifacts = self.root / 'tested artifacts'
        self.artifacts.mkdir()
        self.command('git', 'init', '--bare', str(self.remote))
        self.clone(self.checkout)
        self.git('checkout', '-b', 'main')
        action = self.checkout / 'oci/build-image/action.yml'
        action.parent.mkdir(parents=True)
        action.write_text('  image: docker://example.invalid/oci-build-image@sha256:'+'a'*64+'\n')
        self.git('add', '.')
        self.git('commit', '-m', 'feat: fixture')
        self.git('push', 'origin', 'main')
        self.base = self.git('rev-parse', 'HEAD')
        (self.artifacts/'release-plan.json').write_text(json.dumps({'base': self.base, 'version': '0.1.0'}))
        (self.artifacts/'publication.json').write_text(json.dumps({'version': '0.1.0', 'images': {'oci-build-image': {'signed': True}}}))
        (self.artifacts/'action-digests.patch').write_text('')

    @staticmethod
    def command(*args):
        return subprocess.check_output(args, stderr=subprocess.STDOUT, text=True).strip()

    def clone(self, path):
        self.command('git', 'clone', str(self.remote), str(path))
        for name, value in [('user.name', 'Fixture'), ('user.email', 'fixture@example.invalid'),
                            ('commit.gpgsign', 'false'), ('tag.gpgsign', 'false')]:
            self.command('git', '-C', str(path), 'config', name, value)

    def git(self, *args):
        return self.command('git', '-C', str(self.checkout), *args)

    def finish(self, before_push=None):
        original_run = subprocess.run

        def run(args, **kwargs):
            if args[:3] == ['git', 'push', '--atomic'] and before_push:
                before_push()
            kwargs.setdefault('cwd', self.checkout)
            return original_run(args, **kwargs)

        with patch.object(release, 'ROOT', self.checkout), patch.object(release, 'git', self.git), \
             patch.object(release, 'discover', return_value=({'oci/build-image': 'oci-build-image'}, {})), \
             patch.object(release, 'cliff', return_value='# 0.1.0'), patch.object(release.subprocess, 'run', run):
            release.finish(self.artifacts)

    def test_rejected_atomic_push_leaves_no_partial_refs_and_fresh_retry_succeeds(self):
        hook = self.remote/'hooks/pre-receive'
        hook.write_text('#!/bin/sh\nexit 1\n')
        hook.chmod(0o755)
        before = {p.name: p.read_bytes() for p in self.artifacts.iterdir()}
        with self.assertRaises(subprocess.CalledProcessError):
            self.finish()
        self.assertEqual(self.git('ls-remote', 'origin', 'refs/heads/main').split()[0], self.base)
        self.assertEqual(self.git('ls-remote', 'origin', 'refs/tags/0.1.0'), '')
        hook.unlink()
        self.checkout = self.root/'retry'
        self.clone(self.checkout)
        self.git('checkout', 'main')
        self.finish()
        self.assertEqual(self.git('ls-remote', 'origin', 'refs/heads/main').split()[0], self.git('rev-parse', '0.1.0'))
        self.assertEqual(before, {p.name: p.read_bytes() for p in self.artifacts.iterdir()})

    def test_concurrent_writer_between_preflight_and_push_is_not_overwritten(self):
        winner = self.root/'winner'
        self.clone(winner)
        self.command('git', '-C', str(winner), 'checkout', 'main')
        (winner/'winner.txt').write_text('concurrent change\n')
        self.command('git', '-C', str(winner), 'add', 'winner.txt')
        self.command('git', '-C', str(winner), 'commit', '-m', 'fix: concurrent writer')
        winner_sha = self.command('git', '-C', str(winner), 'rev-parse', 'HEAD')

        def push_winner():
            self.command('git', '-C', str(winner), 'push', 'origin', 'main')

        with self.assertRaises(subprocess.CalledProcessError):
            self.finish(push_winner)
        self.assertEqual(self.git('ls-remote', 'origin', 'refs/heads/main').split()[0], winner_sha)
        self.assertEqual(self.git('ls-remote', 'origin', 'refs/tags/0.1.0'), '')
