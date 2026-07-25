from __future__ import annotations

import json
import logging
from datetime import UTC, datetime
from typing import Any

CORRELATION_FIELDS = {
    "release_version",
    "request_id",
    "task_id",
    "attempt_id",
    "job_id",
    "error_code",
}


class StructuredJsonFormatter(logging.Formatter):
    def format(self, record: logging.LogRecord) -> str:
        fields = getattr(record, "lanverse_fields", {})
        payload: dict[str, Any] = {
            "timestamp": datetime.fromtimestamp(record.created, UTC).isoformat(),
            "level": record.levelname,
            "logger": record.name,
            "event": record.getMessage(),
        }
        if isinstance(fields, dict):
            payload.update(fields)
        return json.dumps(payload, ensure_ascii=False, separators=(",", ":"))


class JobLogger:
    def __init__(self, logger: logging.Logger) -> None:
        self._logger = logger

    def info(self, event: str, **fields: object) -> None:
        unexpected = set(fields) - CORRELATION_FIELDS
        if unexpected:
            raise ValueError(f"unreviewed job log fields: {sorted(unexpected)!r}")
        self._logger.info(event, extra={"lanverse_fields": fields})
