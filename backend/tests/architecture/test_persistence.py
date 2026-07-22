from __future__ import annotations

import os
import subprocess
import unittest
from pathlib import Path

from tools.architecture import find_violations


ROOT = Path(__file__).resolve().parents[3]
BACKEND = ROOT / "backend"


class PersistenceArchitectureTests(unittest.TestCase):
    def test_one_initial_revision_owns_current_business_schema(self) -> None:
        config = (BACKEND / "alembic.ini").read_text()
        migration = BACKEND / "migrations/versions/0001_initial.py"

        self.assertTrue(migration.is_file())
        self.assertIn("migrations/versions", config)
        source = migration.read_text()
        self.assertIn('revision = "platform_0001"', source)
        self.assertIn("down_revision = None", source)
        for schema in ("identity", "catalog"):
            self.assertIn(f'CREATE SCHEMA "{schema}"', source)
            self.assertIn(f'DROP SCHEMA "{schema}"', source)

    def test_initial_revision_contains_only_current_business_tables(self) -> None:
        source = (BACKEND / "migrations/versions/0001_initial.py").read_text()

        for table in (
            "users",
            "invitations",
            "sessions",
            "categories",
            "prompt_templates",
            "generation_examples",
        ):
            self.assertIn(f'"{table}"', source)
        for future_schema in (
            "asset",
            "creation",
            "generation",
            "governance",
            "ingestion",
            "search",
        ):
            self.assertNotIn(f'CREATE SCHEMA "{future_schema}"', source)

    def test_business_logic_is_independent_from_sqlalchemy(self) -> None:
        self.assertEqual(find_violations(BACKEND / "src/thief"), [])

    def test_make_exposes_migration_command(self) -> None:
        result = subprocess.run(
            ["make", "--dry-run", "migrate"],
            cwd=ROOT,
            capture_output=True,
            check=False,
            text=True,
        )

        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn("THIEF_DATABASE_URL=", result.stdout)
        self.assertIn("alembic -c alembic.ini upgrade heads", result.stdout)

    def test_make_preserves_an_external_database_url(self) -> None:
        ci_url = "postgresql+psycopg://thief:thief_ci@localhost:5432/thief"
        result = subprocess.run(
            ["make", "--dry-run", "migrate"],
            cwd=ROOT,
            env={**os.environ, "THIEF_DATABASE_URL": ci_url},
            capture_output=True,
            check=False,
            text=True,
        )

        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn(f"THIEF_DATABASE_URL={ci_url}", result.stdout)

    def test_ci_runs_migrations_and_integration_tests_on_postgres_18(self) -> None:
        workflow = (ROOT / ".github/workflows/ci.yml").read_text()

        for expected in (
            "image: postgres:18.4-bookworm",
            "make migrate",
            "make test-integration",
        ):
            self.assertIn(expected, workflow)


if __name__ == "__main__":
    unittest.main()
