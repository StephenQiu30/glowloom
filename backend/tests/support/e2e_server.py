"""Run the application stack with deterministic registration email delivery.

This module is an executable test harness. It is intentionally kept outside the
application package so production startup cannot enable the fixed verification
code by configuration.
"""

import asyncio

from app.core.config import get_settings
from app.main import app
from app.server import run_server
from tests.support.registration_verification import RecordingRegistrationMailer


async def main() -> None:
    settings = get_settings()
    if settings.environment != "test":
        raise RuntimeError("the E2E server may only run in the test environment")
    app.state.registration_mailer = RecordingRegistrationMailer()
    app.state.registration_code_generator = lambda: "123456"
    await run_server(settings)


if __name__ == "__main__":
    asyncio.run(main())
