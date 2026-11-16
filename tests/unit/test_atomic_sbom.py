"""Selection and hosted qualification contract for the new Atomic scanner."""
from pathlib import Path
import sys
import unittest

ROOT = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(ROOT / "scripts"))
import ci
import qualification


class AtomicSBOMContracts(unittest.TestCase):
    def test_owned_source_selects_both_native_rebuilds(self):
        rows = ci.select(["oci/new-atomic-sbom/scan.go"])["include"]
        owned = [row for row in rows if row["image"] == "oci-new-atomic-sbom"]
        self.assertEqual({row["arch"] for row in owned}, {"amd64", "arm64"})
        self.assertTrue(all(row["rebuild"] for row in owned))
        self.assertTrue(ci.build_dependency(ROOT, "oci/new-atomic-sbom", "oci/new-atomic-sbom/coverage.go"))
        self.assertFalse(ci.build_dependency(ROOT, "oci/new-atomic-sbom", "oci/new-sbom/main.go"))

    def test_real_qualification_is_scoped_to_new_row(self):
        text = (ROOT / ".github/workflows/image.yml").read_text()
        start = text.index("      - name: Export real native Atomic SBOM subject")
        end = text.index("      - name: Check out immutable Atomic recipe fixture", start)
        block = text[start:end]
        self.assertEqual(block.count("if: matrix.image == 'oci-new-atomic-sbom'"), 6)
        self.assertIn("uses: ./workflow/new-test-fixtures", block)
        exporter = (ROOT / "workflow/new-test-fixtures/action.yml").read_text()
        self.assertRegex(exporter, r"image: docker://ghcr\.io/tdesaules/github-actions/workflow-new-test-fixtures@sha256:[a-f0-9]{64}")
        self.assertIn("atomic-recipe: tests/fixtures/atomic/Containerfile", block)
        self.assertIn("uses: ./.candidate-actions/oci/new-atomic-sbom", block)
        positive = block[block.index("      - name: Qualify full native"):block.index("      - name: Reject altered")]
        self.assertNotIn("continue-on-error", positive)
        self.assertIn("architecture: ${{ matrix.arch }}", positive)
        self.assertIn("sha256: ${{ steps.atomic-sbom-fixture.outputs.atomic-sha256 }}", positive)
        self.assertIn("revision: ${{ github.sha }}", positive)
        self.assertIn("ATOMIC_SBOM_FIXTURE_BINDING: ${{ steps.atomic-sbom.outputs.source-binding }}", block)
        self.assertIn("ATOMIC_SBOM_FIXTURE_SHA256:", block)
        self.assertIn("absent-files:", block)
        self.assertNotIn("run:", block)
        self.assertTrue(qualification.applies("        if: matrix.image == 'oci-new-atomic-sbom'", "oci-new-atomic-sbom"))
        self.assertFalse(qualification.applies("        if: matrix.image == 'oci-new-atomic-sbom'", "oci-new-sbom"))

    def test_generic_runtime_can_invoke_version_without_new_dispatch(self):
        runtime = (ROOT / "workflow/test-candidate-runtime/main.go").read_text()
        entrypoint = (ROOT / "oci/new-atomic-sbom/main.go").read_text()
        self.assertIn('versionArg := "--tool-version"', runtime)
        self.assertIn('test("native tool version", "/entrypoint"', runtime)
        self.assertIn('os.Args[1] == "--tool-version"', entrypoint)
        self.assertIn("new-atomic-sbom 1 / Syft 1.54.1", entrypoint)


if __name__ == "__main__":
    unittest.main()
