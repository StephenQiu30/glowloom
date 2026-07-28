import json
import logging
import re
import sys
import time
from collections.abc import Mapping
from typing import Any, cast
from uuid import uuid4

from fastapi import Request
from starlette.middleware.base import BaseHTTPMiddleware, RequestResponseEndpoint
from starlette.responses import Response

SENSITIVE_KEYS = {"authorization", "cookie", "password", "token", "api_key", "prompt", "script"}
CONTROL_CHARACTERS = re.compile(r"[\r\n\t]+")


def sanitize(value: Any, *, key: str = "") -> Any:
    normalized_key = key.lower()
    if any(marker in normalized_key for marker in SENSITIVE_KEYS):
        return "[REDACTED]"
    if isinstance(value, Mapping):
        items = cast(Mapping[object, object], value)
        return {
            str(item_key): sanitize(item_value, key=str(item_key))
            for item_key, item_value in items.items()
        }
    if isinstance(value, (list, tuple)):
        return [sanitize(item) for item in cast(list[object] | tuple[object, ...], value)]
    if isinstance(value, str):
        return CONTROL_CHARACTERS.sub(" ", value)
    return value


class JsonFormatter(logging.Formatter):
    def format(self, record: logging.LogRecord) -> str:
        payload: dict[str, Any] = {
            "timestamp": self.formatTime(record, "%Y-%m-%dT%H:%M:%SZ"),
            "level": record.levelname.lower(),
            "logger": record.name,
            "message": sanitize(record.getMessage()),
        }
        context = getattr(record, "context", None)
        if isinstance(context, Mapping):
            payload.update(sanitize(context))
        return json.dumps(payload, ensure_ascii=False, separators=(",", ":"))


def configure_logging(level: str) -> None:
    handler = logging.StreamHandler(sys.stdout)
    handler.setFormatter(JsonFormatter())
    logging.basicConfig(level=level, handlers=[handler], force=True)


class RequestContextMiddleware(BaseHTTPMiddleware):
    async def dispatch(self, request: Request, call_next: RequestResponseEndpoint) -> Response:
        request_id = request.headers.get("x-request-id") or str(uuid4())
        request.state.request_id = request_id
        started = time.perf_counter()
        response = await call_next(request)
        response.headers["x-request-id"] = request_id
        logging.getLogger("lanverse.http").info(
            "request completed",
            extra={
                "context": {
                    "request_id": request_id,
                    "method": request.method,
                    "path": request.url.path,
                    "status_code": response.status_code,
                    "duration_ms": round((time.perf_counter() - started) * 1000, 2),
                }
            },
        )
        return response
