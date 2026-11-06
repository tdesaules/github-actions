import importlib.util
from pathlib import Path
import unittest

path = Path(__file__).resolve().parents[2] / 'workflow/build-pantavisor/build_config.py'
spec = importlib.util.spec_from_file_location('build_config', path)
config = importlib.util.module_from_spec(spec)
spec.loader.exec_module(config)


class ProducerIdentityTests(unittest.TestCase):
    def test_core_root_identity(self):
        config.validate_producer({'name': 'core'}, 'core')

    def test_k0s_profile_identity_without_root_name(self):
        config.validate_producer({'profiles': {'base': {'name': 'k0s',
                                  'features': ['kubernetes-server']}}}, 'k0s')

    def test_missing_or_mismatched_identity_is_rejected(self):
        for document, project in [({}, 'core'), ({}, 'k0s'),
                                  ({'name': 'k0s'}, 'core'),
                                  ({'name': 'k0s'}, 'k0s'),
                                  ({'profiles': {'base': {'name': 'core'}}}, 'k0s')]:
            with self.subTest(document=document, project=project):
                with self.assertRaises(ValueError):
                    config.validate_producer(document, project)
