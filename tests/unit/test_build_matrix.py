import copy
import sys
import os
import subprocess
import tempfile
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parents[2] / 'scripts'))
import ci


class BuildMatrixTests(unittest.TestCase):
    def test_cached_candidate_is_qualified_but_not_built_again(self):
        job = dict(image='oci-new-sbom', arch='amd64', runner='ubuntu-24.04',
                   rebuild=True, scan=True, **{'artifact-id': '123'})
        builds = ci.build_matrix({'include': [job]})['include']
        self.assertEqual([j['image'] for j in builds], ['workflow-new-vulnerability-review'])
        self.assertTrue(job['rebuild'])

    def test_published_only_and_empty_need_no_builds(self):
        for jobs in ([], [{'image': 'oci-export-image', 'arch': 'amd64',
                           'runner': 'ubuntu-24.04', 'rebuild': False}]):
            self.assertEqual(ci.build_matrix({'include': jobs}), {'include': []})

    def test_scan_helpers_are_unique_and_do_not_expand_qualification(self):
        jobs = [dict(image='oci-new-sbom', arch='amd64', runner='ubuntu-24.04', rebuild=True),
                dict(image='oci-export-image', arch='amd64', runner='ubuntu-24.04', rebuild=False, scan=True),
                dict(image='oci-export-image', arch='arm64', runner='ubuntu-24.04-arm', rebuild=False, scan=True)]
        original = copy.deepcopy(jobs)
        builds = ci.build_matrix({'include': jobs})['include']
        self.assertEqual([(j['image'], j['arch']) for j in builds], [
            ('oci-new-sbom', 'amd64'), ('workflow-new-vulnerability-review', 'amd64'),
            ('workflow-new-vulnerability-review', 'arm64')])
        self.assertTrue(all(j['dependency'] for j in builds[1:]))
        self.assertEqual(jobs, original)

    def test_reviewer_product_is_not_duplicated_or_made_advisory(self):
        job = dict(image='workflow-new-vulnerability-review', arch='amd64',
                   runner='ubuntu-24.04', rebuild=True, scan=True)
        self.assertEqual(ci.build_matrix({'include': [job]}), {'include': [job]})


class IgnoreImpactTests(unittest.TestCase):
    def test_retired_directory_rule_does_not_invalidate_unrelated_images(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            env = dict(os.environ, GIT_CONFIG_NOSYSTEM='1', GIT_CONFIG_GLOBAL='/dev/null',
                       GIT_AUTHOR_NAME='Fixture', GIT_AUTHOR_EMAIL='fixture@example.invalid',
                       GIT_COMMITTER_NAME='Fixture', GIT_COMMITTER_EMAIL='fixture@example.invalid')
            def git(*args):
                return subprocess.check_output(['git', *args], cwd=root, env=env, stderr=subprocess.DEVNULL, text=True).strip()
            git('init', '-b', 'main')
            for name in ('a', 'b'):
                directory = root / 'oci' / name
                directory.mkdir(parents=True)
                (directory / 'action.yml').write_text('runs:\n  using: docker\n  image: docker://example/oci-'+name+'@sha256:'+'a'*64+'\n')
                (directory / 'Containerfile').write_text('FROM scratch AS final\nCOPY oci/'+name+'/ /app/\n')
                (directory / 'main.go').write_text('released')
            (root / '.containerignore').write_text('.source-actions\n')
            git('add', '.'); git('commit', '-m', 'release'); git('tag', '1.0.0')
            baseline = git('rev-parse', 'HEAD')
            (root / '.containerignore').write_text('')
            git('add', '.'); git('commit', '-m', 'remove temporary rule')
            history, _ = ci.unreleased_changes(git('rev-parse', 'HEAD'), root)
            for paths in (['.containerignore'], ['.github/workflows/main.yml']):
                matrix = ci.select(paths, root=root, unreleased=history, baseline=baseline)
                self.assertEqual(len(matrix['include']), 0 if paths == ['.containerignore'] else 4)
                self.assertFalse(any(j['rebuild'] for j in matrix['include']))
            (root / '.containerignore').write_text('oci/a/main.go\n')
            git('add', '.'); git('commit', '-m', 'exclude actual input')
            history, _ = ci.unreleased_changes(git('rev-parse', 'HEAD'), root)
            matrix = ci.select(['.github/workflows/main.yml'], root=root, unreleased=history, baseline=baseline)
            self.assertEqual({j['image'] for j in matrix['include'] if j['rebuild']}, {'oci-a'})
