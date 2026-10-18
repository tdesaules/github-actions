import copy
import importlib.util
import json
from pathlib import Path
import unittest

ROOT = Path(__file__).resolve().parents[2]
SPEC = importlib.util.spec_from_file_location('scan_review', ROOT / 'scripts/scan_review.py')
review = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(review)


class PatchedReviewTests(unittest.TestCase):
    def setUp(self):
        self.policy = json.loads((ROOT / 'grype-review.json').read_text())['patched_npm']
        self.sbom = {'artifacts': [{'id': 'fixture', 'name': 'braces', 'version': '3.0.3',
                                   'type': 'npm', 'locations': [{'path': '/modules/braces/package.json'}]}],
                     'files': [{'location': {'path': '/modules/braces/' + path},
                                'digests': [{'algorithm': 'sha256', 'value': digest}]}
                               for path, digest in self.policy['files'].items()]}

    def test_exact_patch_evidence_is_retained(self):
        rules, evidence = review.patched_npm_review(self.sbom, self.policy)
        self.assertEqual([r['vulnerability'] for r in rules], ['GHSA-vfj7-8cjw-p6xm'])
        self.assertEqual(len(evidence[0]['patched_files']), 3)

    def test_absent_tampered_or_ambiguous_evidence_fails_closed(self):
        for mutation in ('missing', 'changed', 'duplicate', 'version', 'second-copy'):
            sbom = copy.deepcopy(self.sbom)
            if mutation == 'missing':
                sbom['files'].pop()
            elif mutation == 'changed':
                sbom['files'][0]['digests'][0]['value'] = '0' * 64
            elif mutation == 'duplicate':
                sbom['files'].append(copy.deepcopy(sbom['files'][0]))
            elif mutation == 'version':
                sbom['artifacts'][0]['version'] = '3.0.2'
            else:
                other = copy.deepcopy(sbom['artifacts'][0])
                other['locations'] = [{'path': '/other/braces/package.json'}]
                sbom['artifacts'].append(other)
            with self.subTest(mutation=mutation):
                self.assertEqual(review.patched_npm_review(sbom, self.policy), ([], []))
