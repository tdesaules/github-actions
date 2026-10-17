import sys
from pathlib import Path
import unittest

sys.path.insert(0, str(Path(__file__).resolve().parents[2] / 'scripts'))
from resume import validate


class ResumeTests(unittest.TestCase):
    def test_same_main_success_is_required(self):
        run = {'head_sha': 'a'*40, 'head_branch': 'main', 'path': '.github/workflows/main.yml',
               'event': 'push', 'conclusion': 'success'}
        validate(run, 'a'*40)
        for key, value in [('head_sha', 'b'*40), ('head_branch', 'feature'),
                           ('path', 'other.yml'), ('event', 'pull_request'), ('conclusion', 'failure')]:
            with self.subTest(key=key), self.assertRaises(ValueError):
                validate(run | {key: value}, 'a'*40)
