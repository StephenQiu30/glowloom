import json
import logging
import re
import sys
import time
from collections.abc import Mapping
from threading import Lock
from typing import Any, cast
from uuid import UUID

from fastapi import Request
from opentelemetry.trace import SpanKind
from prometheus_client import Counter
from starlette.middleware.base import BaseHTTPMiddleware, RequestResponseEndpoint
from starlette.responses import Response
from uuid6 import uuid7

from app.core.telemetry import (
    span_identifiers,
    start_span,
    traceparent_from_context,
)

MAX_LOG_STRING_LENGTH = 512
SENSITIVE_KEYS = {
    "api_key",
    "authorization",
    "body",
    "cookie",
    "credential",
    "password",
    "presigned_url",
    "prompt",
    "proof",
    "script",
    "secret",
    "set-cookie",
    "token",
}
CONTROL_CHARACTERS = re.compile(r"[\r\n\t]+")
EVENT_ATTRIBUTE_ALLOWLIST: dict[str, frozenset[str]] = {
    "http.request.completed": frozenset(
        {
            "request_id",
            "trace_id",
            "span_id",
            "method",
            "route",
            "status_class",
            "duration_ms",
        }
    ),
    "http.request.failed": frozenset(
        {
            "request_id",
            "trace_id",
            "span_id",
            "method",
            "route",
            "status_class",
            "duration_ms",
            "error_type",
        }
    ),
    "outbox.publish.completed": frozenset(
        {
            "request_id",
            "trace_id",
            "span_id",
            "event_id",
            "event_type",
            "queue",
            "attempt",
            "result",
            "duration_ms",
        }
    ),
    "outbox.publish.failed": frozenset(
        {
            "request_id",
            "trace_id",
            "span_id",
            "event_id",
            "event_type",
            "queue",
            "attempt",
            "result",
            "duration_ms",
            "retryable",
            "error_type",
        }
    ),
    "message.consume.completed": frozenset(
        {
            "request_id",
            "trace_id",
            "span_id",
            "event_id",
            "event_type",
            "queue",
            "result",
            "duration_ms",
        }
    ),
    "message.consume.failed": frozenset(
        {
            "request_id",
            "trace_id",
            "span_id",
            "event_id",
            "event_type",
            "queue",
            "result",
            "duration_ms",
            "retryable",
            "error_type",
        }
    ),
    "storage.operation.completed": frozenset(
        {
            "trace_id",
            "span_id",
            "storage_profile",
            "operation",
            "result",
            "duration_ms",
            "bytes_processed",
        }
    ),
    "storage.operation.failed": frozenset(
        {
            "trace_id",
            "span_id",
            "storage_profile",
            "operation",
            "result",
            "duration_ms",
            "error_code",
        }
    ),
    "media.probe.completed": frozenset(
        {
            "trace_id",
            "span_id",
            "kind",
            "result",
            "duration_ms",
        }
    ),
    "media.probe.failed": frozenset(
        {
            "trace_id",
            "span_id",
            "kind",
            "result",
            "duration_ms",
            "error_code",
        }
    ),
}
TELEMETRY_REDACTION_DROPS = Counter(
    "lanverse_telemetry_redaction_drops_total",
    "Telemetry attributes dropped before export",
    ("signal", "reason"),
)
_LOGGING_CONFIGURATION_LOCK = Lock()
_logging_configured = False


def sanitize(value: Any, *, key: str = "") -> Any:
    normalized_key = key.lower()
    if any(marker in normalized_key for marker in SENSITIVE_KEYS):
        return "[REDACTED]"
    if isinstance(value, Mapping):
        items = cast(Mapping[object, object], value)
        return {
            CONTROL_CHARACTERS.sub(" ", str(item_key))[:MAX_LOG_STRING_LENGTH]: sanitize(
                item_value,
                key=str(item_key),
            )
            for item_key, item_value in items.items()
        }
    if isinstance(value, (list, tuple)):
        return [sanitize(item) for item in cast(list[object] | tuple[object, ...], value)]
    if isinstance(value, str):
        return CONTROL_CHARACTERS.sub(" ", value)[:MAX_LOG_STRING_LENGTH]
    return value


class JsonFormatter(logging.Formatter):
    converter = time.gmtime

    def __init__(
        self,
        *,
        service: str = "lanverse",
        environment: str = "development",
    ) -> None:
        super().__init__()
        self._service = service
        self._environment = environment

    def format(self, record: logging.LogRecord) -> str:
        payload: dict[str, Any] = {
            "timestamp": self.formatTime(record, "%Y-%m-%dT%H:%M:%SZ"),
            "level": record.levelname.lower(),
            "service": self._service,
            "component": record.name,
            "environment": self._environment,
            "event_name": getattr(record, "event_name", "log.message"),
            "message": sanitize(record.getMessage()),
        }
        context = getattr(record, "context", None)
        if isinstance(context, Mapping):
            sanitized_context = sanitize(context)
            if isinstance(sanitized_context, Mapping):
                sanitized_items = cast(Mapping[object, object], sanitized_context)
                for raw_key, value in sanitized_items.items():
                    key = str(raw_key)
                    if key not in payload:
                        payload[key] = value
        return json.dumps(payload, ensure_ascii=False, separators=(",", ":"))


def configure_logging(
    level: str,
    *,
    service: str = "lanverse-api",
    environment: str = "development",
) -> None:
    global _logging_configured
    with _LOGGING_CONFIGURATION_LOCK:
        if _logging_configured:
            return
        handler = logging.StreamHandler(sys.stdout)
        handler.setFormatter(JsonFormatter(service=service, environment=environment))
        logging.basicConfig(level=level, handlers=[handler], force=True)
        _logging_configured = True


def log_event(
    logger: logging.Logger,
    level: int,
    event_name: str,
    message: str,
    **attributes: object,
) -> None:
    allowed = EVENT_ATTRIBUTE_ALLOWLIST.get(event_name)
    if allowed is None:
        raise ValueError("log event is not registered")
    context = {key: value for key, value in attributes.items() if key in allowed}
    dropped = len(attributes) - len(context)
    if dropped:
        TELEMETRY_REDACTION_DROPS.labels(
            signal="log",
            reason="attribute_not_allowed",
        ).inc(dropped)
    logger.log(
        level,
        message,
        extra={"event_name": event_name, "context": sanitize(context)},
    )


def normalize_request_id(value: str | None) -> str:
    if value is not None:
        try:
            parsed = UUID(value)
        except ValueError:
            pass
        else:
            if parsed.version == 7 and str(parsed) == value:
                return value
    return str(uuid7())


def route_template(request: Request) -> str:
    route = request.scope.get("route")
    path = getattr(route, "path", None)
    return path if isinstance(path, str) else "unmatched"


class RequestContextMiddleware(BaseHTTPMiddleware):
    async def dispatch(self, request: Request, call_next: RequestResponseEndpoint) -> Response:
        request_id = normalize_request_id(request.headers.get("x-request-id"))
        request.state.request_id = request_id
        started = time.perf_counter()
        with start_span(
            "http.request",
            kind=SpanKind.SERVER,
            parent_traceparent=request.headers.get("traceparent"),
            attributes={"http.request.method": request.method},
        ) as span:
            trace_id, span_id = span_identifiers(span)
            request.state.trace_id = trace_id
            request.state.span_id = span_id
            try:
                response = await call_next(request)
            except Exception as error:
                route = route_template(request)
                span.set_attribute("http.route", route)
                span.set_attribute("http.response.status_code", 500)
                span.set_attribute("error.type", type(error).__name__)
                log_event(
                    logging.getLogger("lanverse.http"),
                    logging.ERROR,
                    "http.request.failed",
                    "request failed",
                    request_id=request_id,
                    trace_id=trace_id,
                    span_id=span_id,
                    method=request.method,
                    route=route,
                    status_class="5xx",
                    duration_ms=round((time.perf_counter() - started) * 1000, 2),
                    error_type=type(error).__name__,
                )
                raise
            route = route_template(request)
            status_class = f"{response.status_code // 100}xx"
            span.set_attribute("http.route", route)
            span.set_attribute("http.response.status_code", response.status_code)
            response.headers["x-request-id"] = request_id
            response_traceparent = traceparent_from_context()
            if response_traceparent is not None:
                response.headers["traceparent"] = response_traceparent
            log_event(
                logging.getLogger("lanverse.http"),
                logging.INFO,
                "http.request.completed",
                "request completed",
                request_id=request_id,
                trace_id=trace_id,
                span_id=span_id,
                method=request.method,
                route=route,
                status_class=status_class,
                duration_ms=round((time.perf_counter() - started) * 1000, 2),
            )
            return response
