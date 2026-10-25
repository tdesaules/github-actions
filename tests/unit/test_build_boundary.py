import hashlib
import json
from pathlib import Path
import shutil
import tempfile
import unittest
from unittest.mock import patch
import sys

sys.path.insert(0, str(Path(__file__).resolve().parents[2] / 'scripts'))
import images


class BuildBoundaryTests(unittest.TestCase):
    def test_untested_transfer_and_tamper_rejection_before_engine_load(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            archive = root / 'image.oci.tar'
            archive.write_bytes(b'archive transport fixture')
            checksum = hashlib.sha256(archive.read_bytes()).hexdigest()
            with patch.object(images, 'ROOT', root), \
                 patch.object(images, 'architecture', return_value='amd64'), \
                 patch.object(images.subprocess, 'check_output', return_value='a' * 40), \
                 patch.dict(images.os.environ, {'BUILDER_SHA256': checksum}), \
                 patch.object(images, 'load_archive') as load:
                images.build_receipt('check-fixture', 'candidate')
                receipt = json.loads((root / 'build.json').read_text())
                self.assertIs(receipt['tested'], False)
                self.assertFalse((root / 'metadata.json').exists())
                target = root / '.build/check-fixture/amd64'
                target.mkdir(parents=True)
                shutil.copy2(archive, target / 'image.oci.tar')
                shutil.copy2(root / 'build.json', target / 'build.json')
                images.load_build_artifact('check-fixture', 'candidate')
                load.assert_called_once()
                load.reset_mock()
                (target / 'image.oci.tar').write_bytes(b'changed after transfer')
                with self.assertRaisesRegex(ValueError, 'checksum mismatch'):
                    images.load_build_artifact('check-fixture', 'candidate')
                load.assert_not_called()
                shutil.copy2(archive, target / 'image.oci.tar')
                receipt['tested'] = True
                (target / 'build.json').write_text(json.dumps(receipt))
                with self.assertRaisesRegex(ValueError, 'receipt does not match'):
                    images.load_build_artifact('check-fixture', 'candidate')
                load.assert_not_called()
