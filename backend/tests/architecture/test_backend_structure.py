from __future__ import annotations

import ast
import subprocess
import tempfile
import tomllib
import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[3]
BACKEND = ROOT / "backend"
SOURCE = BACKEND / "src/thief"


class BackendStructureTests(unittest.TestCase):
    def test_unimplemented_business_module_templates_are_rejected(self) -> None:
        from tools.architecture import unexpected_module_errors

        with tempfile.TemporaryDirectory() as directory:
            source = Path(directory) / "thief"
            source.mkdir()
            (source / "creation").mkdir()

            self.assertEqual(
                unexpected_module_errors(source),
                ["unexpected-module:creation"],
            )

    def test_backend_uses_one_real_python_package(self) -> None:
        required = {
            "__init__.py",
            "api",
            "catalog",
            "identity",
            "infrastructure",
            "scheduler.py",
            "settings.py",
            "worker.py",
        }

        self.assertTrue(SOURCE.is_dir())
        self.assertTrue(required <= {path.name for path in SOURCE.iterdir()})
        self.assertFalse((BACKEND / "apps").exists())
        self.assertFalse((BACKEND / "packages").exists())

    def test_python_tooling_uses_only_backend_src(self) -> None:
        manifest = tomllib.loads((BACKEND / "pyproject.toml").read_text())
        makefile = (ROOT / "Makefile").read_text()

        self.assertEqual(manifest["tool"]["pytest"]["ini_options"]["pythonpath"], ["src"])
        self.assertEqual(manifest["tool"]["mypy"]["mypy_path"], ["src"])
        self.assertIn("$(CURDIR)/$(BACKEND)/src", makefile)
        for obsolete in ("backend/apps", "backend/packages", "apps/api", "packages/core"):
            self.assertNotIn(obsolete, makefile)

    def test_source_has_no_placeholder_or_oversized_python_files(self) -> None:
        problems: list[str] = []
        for path in sorted(SOURCE.rglob("*.py")):
            source = path.read_text(encoding="utf-8")
            tree = ast.parse(source, filename=str(path))
            statements = list(tree.body)
            if statements and isinstance(statements[0], ast.Expr):
                value = statements[0].value
                if isinstance(value, ast.Constant) and isinstance(value.value, str):
                    statements = statements[1:]
            if not statements:
                problems.append(f"placeholder:{path.relative_to(BACKEND)}")
            if len(source.splitlines()) > 200:
                problems.append(f"oversized:{path.relative_to(BACKEND)}")

        self.assertEqual(problems, [])

    def test_alembic_has_one_linear_initial_revision(self) -> None:
        config = (BACKEND / "alembic.ini").read_text()
        revisions = sorted((BACKEND / "migrations/versions").glob("*.py"))
        result = subprocess.run(
            ["uv", "run", "--directory", "backend", "alembic", "-c", "alembic.ini", "heads"],
            cwd=ROOT,
            capture_output=True,
            check=False,
            text=True,
        )

        self.assertIn("migrations/versions", config)
        self.assertNotIn("migrations/identity/versions", config)
        self.assertEqual([path.name for path in revisions], ["0001_initial.py"])
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(len(result.stdout.strip().splitlines()), 1)

    def test_runtime_entrypoints_use_the_same_package(self) -> None:
        for target, expected in (
            ("run-api", "thief.api.app:app"),
            ("run-worker", "thief.worker:app"),
            ("run-scheduler", "thief.scheduler:app"),
        ):
            result = subprocess.run(
                ["make", "--dry-run", target],
                cwd=ROOT,
                capture_output=True,
                check=False,
                text=True,
            )
            self.assertEqual(result.returncode, 0, result.stderr)
            self.assertIn(expected, result.stdout)


if __name__ == "__main__":
    unittest.main()
