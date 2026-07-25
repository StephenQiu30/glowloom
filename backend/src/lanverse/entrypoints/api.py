from __future__ import annotations

import uvicorn

from lanverse.bootstrap.api import create_app
from lanverse.shared_kernel.config import load_settings
from lanverse.shared_kernel.logging import configure_logging


def main() -> None:
    settings = load_settings()
    configure_logging(settings.log_level)
    uvicorn.run(
        create_app(settings),
        host=settings.api_host,
        port=settings.api_port,
        log_level=settings.log_level.lower(),
    )
