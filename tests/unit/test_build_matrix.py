import copy
import sys
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parents[2] / 'scripts'))
import ci


class BuildMatrixTests(unittest.TestCase):
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
