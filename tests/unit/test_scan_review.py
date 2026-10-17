import sys
from pathlib import Path
import unittest

sys.path.insert(0, str(Path(__file__).resolve().parents[2] / 'scripts'))
from scan_review import review


class ReviewTests(unittest.TestCase):
    policy = {'package': 'github.com/docker/docker', 'affected_package': 'github.com/docker/docker/pkg/authorization',
              'vulnerability': 'GO-2026-4887', 'reference': 'upstream fix', 'reason': 'absent code'}

    def artifact(self, symbols):
        return {'id': 'binary-module', 'name': self.policy['package'], 'version': 'v28.5.1',
                'metadata': {'symbols': symbols}}

    def test_client_symbols_allow_exact_review(self):
        config, evidence = review({'artifacts': [self.artifact({'github.com/docker/docker/client': ['Ping']})]}, self.policy)
        self.assertEqual(config['ignore'][0]['vulnerability'], 'GO-2026-4887')
        self.assertEqual(config['ignore'][0]['package']['version'], 'v28.5.1')
        self.assertEqual(len(evidence), 1)

    def test_missing_symbols_and_linked_daemon_fail_closed(self):
        for symbols in ({}, {'client': []}, {self.policy['affected_package']: ['AuthZRequest']}):
            with self.subTest(symbols=symbols):
                config, _ = review({'artifacts': [self.artifact(symbols)]}, self.policy)
                self.assertEqual(config['ignore'], [])

    def test_one_affected_binary_prevents_module_wide_review(self):
        artifacts = [self.artifact({'client': ['Ping']}), self.artifact({self.policy['affected_package']: ['AuthZRequest']})]
        config, _ = review({'artifacts': artifacts}, self.policy)
        self.assertEqual(config['ignore'], [])
