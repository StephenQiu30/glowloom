from __future__ import annotations

import subprocess
import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[3]
COMPOSE_FILE = ROOT / "infra/compose/compose.yaml"
ENV_FILE = ROOT / "infra/compose/.env.example"


class InfrastructureContractTests(unittest.TestCase):
    def test_compose_pins_current_stable_images_and_healthchecks(self) -> None:
        self.assertTrue(COMPOSE_FILE.is_file())
        self.assertTrue(ENV_FILE.is_file())
        compose = COMPOSE_FILE.read_text()

        for expected in (
            "postgres:18.4-bookworm",
            "rabbitmq:4.3.3-management",
            "quay.io/minio/aistor/minio:RELEASE.2026-06-06T02-44-06Z",
            "pg_isready",
            "rabbitmq-diagnostics",
            "/minio/health/live",
            "      - postgres_data:/var/lib/postgresql\n",
            "postgres_data:",
            "rabbitmq_data:",
            "minio_data:",
        ):
            self.assertIn(expected, compose)

    def test_compose_configuration_is_valid(self) -> None:
        result = subprocess.run(
            [
                "docker",
                "compose",
                "--env-file",
                str(ENV_FILE),
                "-f",
                str(COMPOSE_FILE),
                "config",
                "--quiet",
            ],
            cwd=ROOT,
            capture_output=True,
            check=False,
            text=True,
        )

        self.assertEqual(result.returncode, 0, result.stderr)

    def test_make_exposes_infrastructure_lifecycle(self) -> None:
        for target, expected in (
            ("infra-config", "config --quiet"),
            ("infra-up", "up --detach --wait"),
            ("infra-down", "down"),
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

        verify = subprocess.run(
            ["make", "--dry-run", "verify"],
            cwd=ROOT,
            capture_output=True,
            check=False,
            text=True,
        )
        self.assertEqual(verify.returncode, 0, verify.stderr)
        self.assertIn("config --quiet", verify.stdout)


if __name__ == "__main__":
    unittest.main()
