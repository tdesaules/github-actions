import sys
from pathlib import Path
import unittest

sys.path.insert(0, str(Path(__file__).resolve().parents[2] / "scripts"))
from publish import check_existing


class ResumeTests(unittest.TestCase):
    def test_missing_is_published(self):
        self.assertFalse(check_existing("sha256:a", None, "image:version"))

    def test_identical_is_reused(self):
        self.assertTrue(check_existing("sha256:a", "sha256:a", "image:version"))

    def test_different_is_never_overwritten(self):
        with self.assertRaises(ValueError):
            check_existing("sha256:a", "sha256:b", "image:version")
