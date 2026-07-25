from __future__ import annotations

import asyncio
import logging
import signal

from lanverse.bootstrap.container import create_container
from lanverse.shared_kernel.config import ApplicationSettings


async def run_worker(settings: ApplicationSettings) -> None:
    container = create_container(settings)
    stopped = asyncio.Event()
    loop = asyncio.get_running_loop()

    for signum in (signal.SIGINT, signal.SIGTERM):
        loop.add_signal_handler(signum, stopped.set)

    logging.getLogger(__name__).info(
        "worker host started",
        extra={"environment": container.settings.environment},
    )
    await stopped.wait()
