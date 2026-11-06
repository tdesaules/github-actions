import json
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest
from unittest.mock import patch

sys.path.insert(0, str(Path(__file__).resolve().parents[2] / 'scripts'))
import reports


class AdvisoryReportsTests(unittest.TestCase):
    def test_scanner_outage_remains_an_error_without_aborting_functional_caller(self):
        with tempfile.TemporaryDirectory() as temporary:
            directory = Path(temporary)
            with patch.object(reports, '_reports', side_effect=subprocess.CalledProcessError(1, 'grype')):
                result = reports.reports(directory)
            self.assertEqual(result['status'], 'error')
            self.assertNotIn('severity_counts', result)
            self.assertEqual(json.loads((directory / 'security-status.json').read_text()), result)

    def test_critical_findings_are_reported_without_a_severity_exit_gate(self):
        with tempfile.TemporaryDirectory() as temporary:
            directory = Path(temporary)
            (directory / 'grype.json').write_text(json.dumps({
                'matches': [{'vulnerability': {'severity': 'Critical'}}]}))
            with patch.object(reports, '_reports'):
                result = reports.reports(directory)
            self.assertEqual(result['status'], 'findings')
            self.assertEqual(result['severity_counts'], {'Critical': 1})
