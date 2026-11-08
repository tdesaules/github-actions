import os
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest

sys.path.insert(0, str(Path(__file__).resolve().parents[2] / 'scripts'))
import qualification

WORKFLOW = '''name: main
jobs:
  build:
    runs-on: ubuntu-24.04
  test:
    runs-on: ubuntu-24.04
    steps:
      - name: Test A
        if: matrix.image == 'oci-a'
        uses: ./.candidate-actions/oci/a
        with:
          directory: tests/fixtures/a
          expected: old-a
      - name: Test B
        if: matrix.image == 'oci-b'
        uses: ./.candidate-actions/oci/b
        with:
          directory: tests/fixtures/b
          expected: old-b
  maintenance:
    steps:
      - name: Maintenance old
        uses: ./maintenance/update-dependencies
'''


class QualificationTests(unittest.TestCase):
    def test_workflow_and_fixture_selection(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            env = dict(os.environ, GIT_CONFIG_NOSYSTEM='1', GIT_CONFIG_GLOBAL='/dev/null',
                       GIT_AUTHOR_NAME='Fixture', GIT_AUTHOR_EMAIL='fixture@example.invalid',
                       GIT_COMMITTER_NAME='Fixture', GIT_COMMITTER_EMAIL='fixture@example.invalid')
            def git(*args):
                return subprocess.check_output(['git', *args], cwd=root, env=env, stderr=subprocess.DEVNULL, text=True).strip()
            git('init', '-b', 'main')
            path = root / '.github/workflows/main.yml'
            path.parent.mkdir(parents=True)
            path.write_text(WORKFLOW)
            git('add', '.'); git('commit', '-m', 'baseline'); before = git('rev-parse', 'HEAD')
            actions = {'oci/a': 'oci-a', 'oci/b': 'oci-b'}
            for changed, text, expected in [
                ('.github/workflows/main.yml', WORKFLOW.replace('Maintenance old', 'Maintenance new'), set()),
                ('.github/workflows/main.yml', WORKFLOW.replace('old-a', 'new-a'), {'oci/a'}),
                ('.github/workflows/main.yml', WORKFLOW.replace('ubuntu-24.04', 'ubuntu-24.04-arm'), set(actions)),
                ('tests/fixtures/b/deleted.json', WORKFLOW, {'oci/b'}),
                ('scripts/ci.py', WORKFLOW, set()),
                ('taskfile.yml', WORKFLOW, set()),
            ]:
                with self.subTest(changed=changed, expected=expected):
                    path.write_text(text)
                    self.assertEqual(qualification.select(root, actions, [changed], before, lambda *args: False), expected)

    def test_unknown_conditions_and_missing_graph_remain_conservative(self):
        self.assertTrue(qualification.applies("        if: matrix.image == 'oci-a' || inputs.extra", 'oci-b'))
        with tempfile.TemporaryDirectory() as tmp:
            actions = {'oci/a': 'oci-a'}
            self.assertEqual(qualification.select(Path(tmp), actions, ['.github/workflows/main.yml'], '', lambda *args: False), set(actions))
