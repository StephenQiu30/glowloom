from __future__ import annotations

import json
import subprocess
import tempfile
import tomllib
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
            "backend/src/thief/api/app.py",
            "backend/src/thief/identity/model.py",
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


class DeliveryWorkflowTests(unittest.TestCase):
    def test_backend_pins_current_argon2_password_library(self) -> None:
        manifest = tomllib.loads((ROOT / "backend/pyproject.toml").read_text())

        self.assertIn("argon2-cffi==25.1.0", manifest["project"]["dependencies"])

    def test_make_verify_runs_current_s0_gates(self) -> None:
        result = subprocess.run(
            ["make", "--dry-run", "verify"],
            cwd=ROOT,
            capture_output=True,
            check=False,
            text=True,
        )

        self.assertEqual(result.returncode, 0, result.stderr)
        for command in (
            "unittest discover",
            "pnpm --dir frontend lint",
            "ruff check",
            "pnpm --dir frontend typecheck",
            "mypy",
            "pnpm --dir frontend build",
            "compileall",
        ):
            self.assertIn(command, result.stdout)

    def test_ci_uses_pinned_toolchains_and_make_verify(self) -> None:
        workflow = (ROOT / ".github/workflows/ci.yml").read_text()

        for expected in (
            'node-version-file: ".node-version"',
            'python-version-file: ".python-version"',
            'version: "0.11.31"',
            "pnpm@11.15.1",
            "make bootstrap",
            "make verify",
        ):
            self.assertIn(expected, workflow)

        action_lines = [
            line.strip() for line in workflow.splitlines() if "uses:" in line
        ]
        self.assertTrue(action_lines)
        for line in action_lines:
            reference = line.rsplit("@", 1)[-1].split()[0]
            self.assertEqual(len(reference), 40, line)


class ArchitectureBoundaryTests(unittest.TestCase):
    def test_current_backend_has_no_boundary_violations(self) -> None:
        source = ROOT / "backend/src/thief"

        self.assertEqual(find_violations(source), [])

    def test_business_logic_rejects_framework_imports(self) -> None:
        violations = self._scan("catalog/model.py", "from fastapi import FastAPI\n")

        self.assert_rule(violations, "business-framework")

    def test_business_module_rejects_other_business_module_imports(self) -> None:
        violations = self._scan(
            "identity/sessions.py",
            "from thief.catalog.model import PromptTemplate\n",
        )

        self.assert_rule(violations, "cross-business-module")

    def test_api_may_compose_business_modules_and_frameworks(self) -> None:
        violations = self._scan("api/app.py", "from fastapi import FastAPI\n")

        self.assertEqual(violations, [])

    def _scan(self, relative_path: str, content: str) -> list[str]:
        with tempfile.TemporaryDirectory() as directory:
            source = Path(directory) / "thief"
            target = source / relative_path
            target.parent.mkdir(parents=True)
            target.write_text(content, encoding="utf-8")
            return find_violations(source)

    def assert_rule(self, violations: list[str], rule: str) -> None:
        self.assertTrue(
            any(item.startswith(f"{rule}:") for item in violations),
            violations,
        )


if __name__ == "__main__":
    unittest.main()
