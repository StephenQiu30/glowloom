from __future__ import annotations

import logging

from lanverse.jobs.observability import StructuredJsonFormatter


def configure_logging(level: str) -> None:
    handler = logging.StreamHandler()
    handler.setFormatter(StructuredJsonFormatter())
    logging.basicConfig(level=level, handlers=[handler], force=True)
