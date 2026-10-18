import hashlib
import importlib.util
import json
from pathlib import Path
import tempfile
import unittest
import sys
import subprocess
from unittest.mock import patch

sys.path.insert(0, str(Path(__file__).resolve().parents[2] / "scripts"))
from testing import fixture_archive

SPEC = importlib.util.spec_from_file_location("ci", Path(__file__).resolve().parents[2] / "scripts/ci.py")
ci = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(ci)


class SelectionTests(unittest.TestCase):
    def test_real_git_rename_and_deletion_with_spaces(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)

            def git(*args):
                return subprocess.check_output(["git", *args], cwd=root, stderr=subprocess.DEVNULL).decode().strip()

            git("init", "-b", "main")
            git("config", "user.email", "test@example.invalid")
            git("config", "user.name", "Fixture")
            (root / "old name").write_text("same content\n")
            (root / "deleted").write_text("removed\n")
            git("add", ".")
            git("commit", "-m", "feat: initial fixture")
            before = git("rev-parse", "HEAD")
            (root / "old name").rename(root / "new name")
            (root / "deleted").unlink()
            git("add", "-A")
            git("commit", "-m", "fix: rename and remove")
            after = git("rev-parse", "HEAD")
            with patch.object(ci, "ROOT", root):
                self.assertEqual(set(ci.changed_paths(before, after)), {"old name", "new name", "deleted"})

    def test_documentation_only_has_empty_matrix(self):
        self.assertEqual(ci.select(["README.md", "docs/bootstrap.md"]), {"include": []})

    def test_shared_change_selects_both_architectures(self):
        jobs = ci.select(["shared/runtime/main.go"])["include"]
        self.assertEqual(len(jobs), 30)
        self.assertTrue(all(j["rebuild"] for j in jobs))
        self.assertEqual({j["arch"] for j in jobs}, {"amd64", "arm64"})

    def test_action_change_does_not_rebuild(self):
        jobs = ci.select(["oci/new-sbom/action.yml"])["include"]
        self.assertEqual(len(jobs), 2)
        self.assertTrue(all(not j["rebuild"] for j in jobs))

    def test_removed_component_not_scheduled(self):
        self.assertEqual(ci.select(["oci/deleted/Containerfile"]), {"include": []})

    def test_rename_selects_both_sides(self):
        jobs = ci.select(["oci/new-sbom/Containerfile", "oci/publish-image/Containerfile"])["include"]
        self.assertEqual({j["image"] for j in jobs}, {"oci-new-sbom", "oci-publish-image"})

    def test_first_release_full_selection(self):
        self.assertEqual(len(ci.select([], full=True)["include"]), 30)

    def test_action_main_rebuilds_only_its_image(self):
        jobs = ci.select(['oci/export-image/main.go'])['include']
        self.assertEqual({j['image'] for j in jobs}, {'oci-export-image'})
        self.assertTrue(all(j['rebuild'] for j in jobs))

    def test_check_is_an_independent_image(self):
        jobs = ci.select(['check/test-yaml/main.go'])['include']
        self.assertEqual({j['image'] for j in jobs}, {'check-test-yaml'})
        self.assertEqual({j['arch'] for j in jobs}, {'amd64', 'arm64'})
        self.assertTrue(all(j['rebuild'] for j in jobs))


class ModesTests(unittest.TestCase):
    def test_modes(self):
        self.assertEqual(ci.mode("push", "refs/heads/feature"), "development")
        self.assertEqual(ci.mode("workflow_dispatch", "refs/heads/main"), "prepare")
        self.assertEqual(ci.mode("push", "refs/tags/1.2.3"), "release")
        self.assertEqual(ci.mode("schedule", "refs/heads/main"), "maintenance")

    def test_reject_invalid_versions_and_ref(self):
        for tag in ("v1.2.3", "01.2.3", "1.2", "1.2.3-01", "1.2.3-", "1.2.3+", "1.2.3\n"):
            with self.subTest(tag=tag), self.assertRaises(ValueError):
                ci.mode("push", "refs/tags/"+tag)
        with self.assertRaises(ValueError):
            ci.mode("workflow_dispatch", "refs/heads/feature")

    def test_bootstrap_and_prerelease_semver(self):
        for tag in ("0.0.0-bootstrap", "1.2.3-rc.1", "1.2.3+build.123"):
            self.assertTrue(ci.SEMVER.fullmatch(tag))


class AggregationTests(unittest.TestCase):
    def test_missing_architecture_duplicate_and_corruption(self):
        with tempfile.TemporaryDirectory() as tmp:
            paths = []
            for arch in ("amd64", "arm64"):
                directory = Path(tmp) / arch
                directory.mkdir()
                fixture_archive(directory / "image.oci.tar", arch)
                checksum = hashlib.sha256((directory / "image.oci.tar").read_bytes()).hexdigest()
                path = directory / "metadata.json"
                path.write_text(json.dumps({"image": "syft", "arch": arch, "tested": True,
                                           "archive_sha256": checksum}))
                paths.append(path)
            with self.assertRaises(ValueError):
                ci.aggregate(paths[:1])
            self.assertEqual([i["arch"] for i in ci.aggregate(paths[::-1])], ["amd64", "arm64"])
            with self.assertRaises(ValueError):
                ci.aggregate(paths + paths)
            swapped = json.loads(paths[0].read_text())
            swapped["arch"] = "arm64"
            paths[0].write_text(json.dumps(swapped))
            with self.assertRaises(ValueError):
                ci.aggregate(paths)
            (paths[0].parent / "image.oci.tar").write_bytes(b"modified")
            with self.assertRaises(ValueError):
                ci.aggregate(paths)


if __name__ == "__main__":
    unittest.main()
