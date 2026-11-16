import os
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest

sys.path.insert(0, str(Path(__file__).resolve().parents[2] / 'scripts'))
import ci


class ReleaseMetadataTests(unittest.TestCase):
    def test_only_tagged_reference_promotions_skip_development_images(self):
        for scenario in ('pins', 'source-promotion', 'interface', 'source', 'untagged', 'wrong-tag', 'wrong-subject', 'range'):
            with self.subTest(scenario=scenario), tempfile.TemporaryDirectory() as tmp:
                root = Path(tmp)
                env = dict(os.environ, GIT_CONFIG_NOSYSTEM='1', GIT_CONFIG_GLOBAL='/dev/null',
                           GIT_AUTHOR_NAME='Fixture', GIT_AUTHOR_EMAIL='fixture@example.invalid',
                           GIT_COMMITTER_NAME='Fixture', GIT_COMMITTER_EMAIL='fixture@example.invalid')
                def git(*args):
                    return subprocess.check_output(['git', '-c', 'commit.gpgsign=false', '-c',
                                                    'tag.gpgsign=false', *args],
                                                   cwd=root, env=env, stderr=subprocess.DEVNULL).decode().strip()
                directory = root / 'check/tool'
                directory.mkdir(parents=True)
                prefix = 'ghcr.io/tdesaules/github-actions/check-tool@sha256:'
                old = 'name: tool\nruns:\n  using: docker\n  image: docker://'+prefix+'a'*64+'\n'
                if scenario == 'source-promotion':
                    old = 'name: tool\nruns:\n  using: docker\n  image: Containerfile\n'
                (directory / 'action.yml').write_text(old)
                (directory / 'Containerfile').write_text(
                    'FROM scratch AS final\nLABEL org.opencontainers.image.title="check-tool"\n')
                git('init', '-b', 'main')
                git('add', '.')
                git('commit', '-m', 'feat: tool')
                before = git('rev-parse', 'HEAD')
                if scenario == 'range':
                    (root / 'extra').write_text('source')
                    git('add', '.')
                    git('commit', '-m', 'fix: unrelated')
                new = 'name: tool\nruns:\n  using: docker\n  image: docker://'+prefix+'b'*64+'\n'
                if scenario == 'interface':
                    new += '  args: [--changed]\n'
                (directory / 'action.yml').write_text(new)
                (root / 'CHANGELOG.md').write_text('# Changelog\n')
                if scenario == 'source':
                    (directory / 'main.go').write_text('package main\n')
                git('add', '.')
                git('commit', '-m', 'fix: other' if scenario == 'wrong-subject' else 'chore(release): 1.2.3')
                head = git('rev-parse', 'HEAD')
                if scenario != 'untagged':
                    git('tag', '1.2.3', before if scenario == 'wrong-tag' else head)
                self.assertEqual(ci.release_metadata_commit(before, head, root),
                                 scenario in ('pins', 'source-promotion'))

    def test_fedora_exception_does_not_allow_other_floating_bases(self):
        for reference in ('quay.io/fedora/fedora:44', 'quay.io/fedora/fedora:45',
                          'quay.io/fedora/fedora-minimal:45'):
            ci.validate_base(reference, set(), 'fixture')
        for reference in ('quay.io/fedora/fedora:latest', 'quay.io/fedora/fedora:rawhide',
                          'quay.io/fedora/fedora:45.1', 'docker.io/library/alpine:3.24'):
            with self.subTest(reference=reference), self.assertRaises(ValueError):
                ci.validate_base(reference, set(), 'fixture')
