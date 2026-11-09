import importlib.util
from pathlib import Path
import unittest
from unittest.mock import patch

ROOT = Path(__file__).resolve().parents[2]
spec = importlib.util.spec_from_file_location('pantavisor_consumers', ROOT / 'scripts/pantavisor_consumers.py')
module = importlib.util.module_from_spec(spec)
spec.loader.exec_module(module)


class ConsumerTests(unittest.TestCase):
    def test_six_jobs_without_host_commands_or_unpublished_pins(self):
        import re
        for project in ('core', 'k0s', 'system'):
            text = module.render(project, '1.5.0')
            self.assertEqual(re.findall(r'^  ([a-z]+):$', text, re.M)[-6:],
                             ['check', 'lint', 'build', 'test', 'release', 'maintenance'])
            self.assertNotRegex(text, r'(?m)^\s+run:')
            self.assertNotIn('pull_request:', text)
            self.assertNotIn(':unpublished', text)
            self.assertNotIn('get-image-matrix@', text)
            self.assertNotIn('__CATALOG_TAG__', text)
            self.assertIn('read-pantavisor-release@1.5.0', text)
            self.assertIn('receipt-digest: ${{ steps.provenance.outputs.receipt-digest }}', text)

    def test_system_requires_both_profiles_before_finalization(self):
        text = module.render('system', '1.5.0')
        self.assertIn('profile: [base, k0s]', text)
        self.assertIn('fail-fast: false', text)
        self.assertIn('merge-pantavisor-qualification@1.5.0', text)
        self.assertLess(text.index('merge-pantavisor-qualification@'), text.index('complete-pantavisor-release@'))
        self.assertNotIn('oci/new-rootfs-image@', text)
        self.assertNotIn('oci/publish-image@', text)
        self.assertIn('registry: ghcr.io/${{ github.repository }}/bsp', text)

    def test_publication_checkout_preserves_annotated_tag(self):
        for project in ('core', 'k0s', 'system'):
            release = module.render(project, '1.6.1').split('  release:\n', 1)[1].split('  maintenance:\n', 1)[0]
            checkout = release.split('      - name: Validate release tag and preparation run', 1)[0]
            self.assertIn("ref: ${{ github.ref_type == 'tag' && github.ref || github.sha }}", checkout)
            self.assertIn('fetch-depth: 0', checkout)

    def test_missing_catalogue_release_prevents_activation(self):
        import tempfile
        with tempfile.TemporaryDirectory() as directory:
            identity = Path(directory) / 'build/component.json'
            identity.parent.mkdir()
            identity.write_text('{"name":"core","repository":"fixture/pantavisor-core"}')
            with patch.object(module, 'verify_catalogue', side_effect=ValueError('unpublished')):
                with self.assertRaisesRegex(ValueError, 'unpublished'):
                    module.activate('core', '1.5.0', directory)
            self.assertEqual([p.name for p in Path(directory).iterdir()], ['build'])

    def test_invalid_or_floating_tags_rejected(self):
        for tag in ('main', 'latest', 'v1.5.0', '01.5.0', '1.5.0-beta.1'):
            with self.assertRaises(ValueError):
                module.render('core', tag)

    def test_k0s_activation_checks_base_profile_identity(self):
        import json
        import tempfile
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            identity = root / 'build/component.json'
            identity.parent.mkdir()
            for name in ('core', 'k0s'):
                identity.write_text(json.dumps({'repository': 'fixture/pantavisor-k0s',
                                               'profiles': {'base': {'name': name}}}))
                with patch.object(module, 'verify_catalogue') as verify:
                    if name == 'core':
                        with self.assertRaisesRegex(ValueError, 'Producer identity'):
                            module.activate('k0s', '1.5.0', root)
                        verify.assert_not_called()
                        self.assertFalse((root / '.github').exists())
                    else:
                        module.activate('k0s', '1.5.0', root)
                        self.assertIn('PROJECT: k0s', (root / '.github/workflows/main.yml').read_text())

    def test_activation_preserves_originals_and_refuses_local_edits(self):
        import tempfile
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            (root / 'build').mkdir()
            (root / 'build/component.json').write_text('{"name":"core","repository":"fixture/pantavisor-core"}')
            workflows = root / '.github/workflows'
            workflows.mkdir(parents=True)
            original = '# pre-existing user workflow changes\nname: original\n'
            (workflows / 'check.yml').write_text(original)
            with patch.object(module, 'verify_catalogue'):
                module.activate('core', '1.5.0', root)
                self.assertEqual((root / '.github/legacy-workflows/check.yml').read_text(), original)
                self.assertFalse((workflows / 'check.yml').exists())
                self.assertIn('name: main', (workflows / 'main.yml').read_text())
                module.activate('core', '1.5.0', root)
                edited = (workflows / 'main.yml').read_text() + '\n# manual adjustment\n'
                (workflows / 'main.yml').write_text(edited)
                with self.assertRaisesRegex(ValueError, 'local edits'):
                    module.activate('core', '1.5.0', root)
                self.assertEqual((workflows / 'main.yml').read_text(), edited)
