from __future__ import annotations

import subprocess
import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[3]
CI_WORKFLOW = ROOT / ".github/workflows/ci.yml"
FRONTEND_PACKAGE = ROOT / "frontend/package.json"


class RuntimeContractTests(unittest.TestCase):
    def test_runtime_smoke_passes_an_explicit_database_url(self) -> None:
        result = subprocess.run(
            ["make", "--dry-run", "test-runtime"],
            cwd=ROOT,
            capture_output=True,
            check=False,
            text=True,
        )

        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn("THIEF_DATABASE_URL=", result.stdout)

    def test_production_runtime_packages_assets_and_temp_state(self) -> None:
        package = FRONTEND_PACKAGE.read_text()
        self.assertIn(
            "cp -R .next/static .next/standalone/.next/static",
            package,
        )

        scheduler = subprocess.run(
            ["make", "--dry-run", "run-scheduler"],
            cwd=ROOT,
            capture_output=True,
            check=False,
            text=True,
        )
        self.assertEqual(scheduler.returncode, 0, scheduler.stderr)
        self.assertIn("tmp/celerybeat-schedule", scheduler.stdout)

    def test_runtime_entrypoints_and_smoke_test_exist(self) -> None:
        for path in (
            "backend/src/thief/api/app.py",
            "backend/src/thief/scheduler.py",
            "backend/src/thief/worker.py",
            "backend/tools/runtime_smoke.py",
        ):
            self.assertTrue((ROOT / path).is_file(), path)

        for target, expected in (
            ("run-web", "pnpm --dir frontend start"),
            ("run-api", "uvicorn thief.api.app:app"),
            ("run-worker", "thief.worker:app"),
            ("run-scheduler", "thief.scheduler:app"),
            ("test-runtime", "tools/runtime_smoke.py"),
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

    def test_ci_runs_runtime_smoke_with_rabbitmq(self) -> None:
        workflow = CI_WORKFLOW.read_text()

        for expected in (
            "image: rabbitmq:4.3.3-management",
            "THIEF_RABBITMQ_URL: amqp://thief:thief_ci@localhost:5672//",
            "make test-runtime",
        ):
            self.assertIn(expected, workflow)


if __name__ == "__main__":
    unittest.main()
