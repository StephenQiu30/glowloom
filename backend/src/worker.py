from __future__ import annotations

import asyncio
import logging
import signal

from core.config import ApplicationSettings, load_settings
from core.logging import configure_logging
from core.runtime import create_runtime


async def run_worker(settings: ApplicationSettings) -> None:
    runtime = create_runtime(settings)
    stopped = asyncio.Event()
    loop = asyncio.get_running_loop()
    for signum in (signal.SIGINT, signal.SIGTERM):
        loop.add_signal_handler(signum, stopped.set)

    logging.getLogger(__name__).info(
        "worker host started",
        extra={"environment": runtime.settings.environment},
    )
    await runtime.start()
    try:
        await stopped.wait()
    finally:
        await runtime.close()


def run() -> None:
    settings = load_settings()
    configure_logging(settings.log_level)
    asyncio.run(run_worker(settings))
