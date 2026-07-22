from __future__ import annotations

import json
import tempfile
import unittest
from pathlib import Path

from tools.architecture import find_violations, required_layout_errors


ROOT = Path(__file__).resolve().parents[3]


class FoundationLayoutTests(unittest.TestCase):
    def test_required_foundation_layout_exists(self) -> None:
        self.assertEqual(required_layout_errors(ROOT), [])

    def test_frontend_and_backend_are_top_level_boundaries(self) -> None:
        required = [
            "frontend/package.json",
            "frontend/pnpm-lock.yaml",
            "frontend/src/app/page.tsx",
            "backend/pyproject.toml",
            "backend/uv.lock",
            "backend/apps/api/src/thief_api/main.py",
            "backend/packages/core/src/thief_core/__init__.py",
        ]

        self.assertEqual(
            [path for path in required if not (ROOT / path).is_file()],
            [],
        )
        self.assertFalse((ROOT / "apps").exists())
        self.assertFalse((ROOT / "packages").exists())

    def test_frontend_uses_standard_next_and_radix_shadcn(self) -> None:
        required = [
            "frontend/components.json",
            "frontend/postcss.config.mjs",
            "frontend/src/app/error.tsx",
            "frontend/src/app/loading.tsx",
            "frontend/src/app/not-found.tsx",
            "frontend/src/components/ui/button.tsx",
            "frontend/src/components/ui/card.tsx",
            "frontend/src/lib/utils.ts",
        ]
        missing = [path for path in required if not (ROOT / path).is_file()]
        self.assertEqual(missing, [])

        manifest = json.loads((ROOT / "frontend/package.json").read_text())
        self.assertIn("radix-ui", manifest["dependencies"])


class ArchitectureBoundaryTests(unittest.TestCase):
    def test_current_core_has_no_violations(self) -> None:
        core = ROOT / "backend/packages/core/src/thief_core"

        self.assertEqual(find_violations(core), [])

    def test_core_rejects_framework_imports(self) -> None:
        violations = self._scan(
            "catalog/domain/model.py",
            "from fastapi import FastAPI\n",
        )

        self.assert_rule(violations, "core-framework")

    def test_module_rejects_other_module_internals(self) -> None:
        violations = self._scan(
            "identity/application/use_case.py",
            "from thief_core.catalog.domain.model import Template\n",
        )

        self.assert_rule(violations, "cross-module-internal")

    def test_shared_rejects_business_symbols(self) -> None:
        violations = self._scan(
            "shared/job.py",
            "class GenerationJob:\n    pass\n",
        )

        self.assert_rule(violations, "shared-business-symbol")

    def test_public_contract_import_is_allowed(self) -> None:
        violations = self._scan(
            "identity/application/use_case.py",
            "from thief_core.catalog import PublishTemplate\n",
        )

        self.assertEqual(violations, [])

    def _scan(self, relative_path: str, source: str) -> list[str]:
        with tempfile.TemporaryDirectory() as directory:
            core = Path(directory) / "packages/core/src/thief_core"
            target = core / relative_path
            target.parent.mkdir(parents=True)
            target.write_text(source, encoding="utf-8")
            return find_violations(core)

    def assert_rule(self, violations: list[str], rule: str) -> None:
        self.assertTrue(
            any(item.startswith(f"{rule}:") for item in violations),
            violations,
        )


if __name__ == "__main__":
    unittest.main()
