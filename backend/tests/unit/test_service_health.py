from __future__ import annotations

import asyncio
import contextlib
import io
import os
import subprocess
import sys
import unittest
from pathlib import Path
from unittest.mock import patch

import httpx


ROOT = Path(__file__).resolve().parents[3]
BACKEND = ROOT / "backend"
for source in (
    "apps/api/src",
    "apps/worker/src",
    "apps/scheduler/src",
):
    sys.path.insert(0, str(BACKEND / source))


class ServiceHealthTests(unittest.TestCase):
    def test_api_health_endpoints_follow_the_common_contract(self) -> None:
        asyncio.run(self._assert_api_health_endpoints())

    async def _assert_api_health_endpoints(self) -> None:
        from thief_api.main import app

        transport = httpx.ASGITransport(app=app)
        async with httpx.AsyncClient(
            transport=transport,
            base_url="http://test",
        ) as client:
            for path in ("/health/live", "/health/ready"):
                response = await client.get(path)
                self.assertEqual(response.status_code, 200)
                self.assertEqual(
                    response.json(),
                    {"service": "api", "status": "ok"},
                )

    def test_worker_and_scheduler_expose_cli_healthchecks(self) -> None:
        from thief_scheduler.main import main as scheduler_main
        from thief_worker.app import main as worker_main

        for service, entrypoint in (
            ("worker", worker_main),
            ("scheduler", scheduler_main),
        ):
            output = io.StringIO()
            with contextlib.redirect_stdout(output):
                exit_code = entrypoint(["--healthcheck"])

            self.assertEqual(exit_code, 0)
            self.assertEqual(
                output.getvalue().strip(),
                f'{{"service": "{service}", "status": "ok"}}',
            )

    def test_services_share_the_local_rabbitmq_contract(self) -> None:
        from thief_scheduler.settings import SchedulerSettings
        from thief_worker.settings import WorkerSettings

        with patch.dict(os.environ, {}, clear=True):
            for settings_type in (WorkerSettings, SchedulerSettings):
                settings = settings_type.from_env()
                self.assertEqual(
                    settings.rabbitmq_url,
                    "amqp://thief:thief_local@localhost:5672//",
                )

    def test_production_services_require_an_explicit_broker_url(self) -> None:
        from thief_scheduler.settings import SchedulerSettings
        from thief_worker.settings import WorkerSettings

        with patch.dict(os.environ, {"THIEF_ENV": "production"}, clear=True):
            for settings_type in (WorkerSettings, SchedulerSettings):
                with self.assertRaisesRegex(RuntimeError, "THIEF_RABBITMQ_URL"):
                    settings_type.from_env()

    def test_scheduler_celery_app_uses_its_broker_settings(self) -> None:
        from thief_scheduler.app import app, settings

        self.assertEqual(app.conf.broker_url, settings.rabbitmq_url)

    def test_make_verify_includes_unit_health_tests(self) -> None:
        result = subprocess.run(
            ["make", "--dry-run", "verify"],
            cwd=ROOT,
            capture_output=True,
            check=False,
            text=True,
        )

        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn("unittest discover", result.stdout)
        self.assertIn("tests/unit", result.stdout)


if __name__ == "__main__":
    unittest.main()
