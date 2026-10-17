import importlib.util
import json
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest
from unittest.mock import patch


sys.path.insert(0, str(Path(__file__).resolve().parents[2] / 'scripts'))
SPEC = importlib.util.spec_from_file_location('catalogue_release', Path(__file__).resolve().parents[2] / 'scripts/release.py')
release = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(release)


class ReleaseTests(unittest.TestCase):
    def test_initial_plan_and_no_change(self):
        def git(*args):
            if args[0] == 'rev-parse':
                return 'a'*40
            if args[0] == 'tag':
                return ''
            return 'feat: initial catalogue'
        with patch.object(release, 'git', git), patch.object(release, 'cliff', return_value='0.1.0'):
            self.assertEqual(release.plan(), {'base': 'a'*40, 'previous': None, 'version': '0.1.0', 'changed': True})
        with patch.object(release, 'git', side_effect=['a'*40, '0.1.0', 'chore(release): 0.1.0']), patch.object(release, 'cliff') as cliff:
            self.assertFalse(release.plan()['changed'])
            cliff.assert_not_called()

    def test_atomic_release_and_advanced_main(self):
        with tempfile.TemporaryDirectory(prefix='release regression ') as temporary:
            root = Path(temporary)
            remote, checkout, artifacts = root/'remote.git', root/'checkout', root/'artifacts'
            artifacts.mkdir()
            subprocess.run(['git', 'init', '--bare', str(remote)], check=True, capture_output=True)
            subprocess.run(['git', 'clone', str(remote), str(checkout)], check=True, capture_output=True)

            def git(*args):
                return subprocess.check_output(['git', '-C', str(checkout), *args], stderr=subprocess.DEVNULL, text=True).strip()

            git('config', 'user.name', 'Test')
            git('config', 'user.email', 'test@example.invalid')
            git('config', 'commit.gpgsign', 'false')
            git('config', 'tag.gpgsign', 'false')
            git('checkout', '-b', 'main')
            action = checkout/'oci/build-image/action.yml'
            action.parent.mkdir(parents=True)
            action.write_text('  image: docker://example.invalid/buildah@sha256:'+'a'*64+'\n')
            git('add', '.')
            git('commit', '-m', 'feat: initial')
            git('push', 'origin', 'main')
            base = git('rev-parse', 'HEAD')
            (artifacts/'release-plan.json').write_text(json.dumps({'base': base, 'version': '0.1.0'}))
            (artifacts/'publication.json').write_text(json.dumps({'version': '0.1.0', 'images': {'buildah': {'signed': True}}}))
            (artifacts/'action-digests.patch').write_text('')
            original_run = subprocess.run

            def run(args, **kwargs):
                kwargs.setdefault('cwd', checkout)
                return original_run(args, **kwargs)

            with patch.object(release, 'ROOT', checkout), patch.object(release, 'git', git), \
                 patch.object(release, 'discover', return_value=({'oci/build-image': 'buildah'}, {})), \
                 patch.object(release, 'cliff', return_value='# 0.1.0'), patch.object(release.subprocess, 'run', run):
                release.finish(artifacts)
                tag = git('rev-parse', '0.1.0')
                self.assertEqual(tag, git('rev-parse', 'HEAD'))
                self.assertIn(tag, git('ls-remote', 'origin', 'refs/heads/main'))
                self.assertIn(tag, git('ls-remote', 'origin', 'refs/tags/0.1.0'))
                git('checkout', '--detach', base)
                with self.assertRaisesRegex(ValueError, 'main advanced'):
                    release.finish(artifacts)
