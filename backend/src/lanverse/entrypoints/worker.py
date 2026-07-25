from __future__ import annotations

import asyncio

from lanverse.bootstrap.worker import run_worker
from lanverse.shared_kernel.config import load_settings
from lanverse.shared_kernel.logging import configure_logging


def main() -> None:
    settings = load_settings()
    configure_logging(settings.log_level)
    asyncio.run(run_worker(settings))
