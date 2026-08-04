from collections.abc import Generator, Mapping, Sequence
from contextlib import contextmanager
from threading import Lock
from typing import cast

from opentelemetry import trace
from opentelemetry.context import Context
from opentelemetry.sdk.resources import Resource
from opentelemetry.sdk.trace import TracerProvider
from opentelemetry.trace import Span, SpanKind, Tracer
from opentelemetry.trace.propagation.tracecontext import TraceContextTextMapPropagator

TRACEPARENT_HEADER = "traceparent"
_INSTRUMENTATION_NAME = "lanverse.backend"
_PROPAGATOR = TraceContextTextMapPropagator()
_provider: TracerProvider | None = None
_PROVIDER_LOCK = Lock()
SpanAttribute = (
    str | bool | int | float | Sequence[str] | Sequence[bool] | Sequence[int] | Sequence[float]
)


def configure_telemetry(
    *,
    service_name: str,
    environment: str,
) -> TracerProvider:
    global _provider
    with _PROVIDER_LOCK:
        if _provider is not None:
            return _provider
        candidate = TracerProvider(
            resource=Resource.create(
                {
                    "service.name": service_name,
                    "service.version": "0.1.0",
                    "deployment.environment": environment,
                }
            )
        )
        trace.set_tracer_provider(candidate)
        installed = trace.get_tracer_provider()
        _provider = installed if isinstance(installed, TracerProvider) else candidate
        return _provider


def get_tracer() -> Tracer:
    provider = _provider or configure_telemetry(
        service_name="lanverse-backend",
        environment="development",
    )
    return provider.get_tracer(_INSTRUMENTATION_NAME)


def traceparent_from_context(context: Context | None = None) -> str | None:
    carrier: dict[str, str] = {}
    _PROPAGATOR.inject(carrier, context=context)
    return carrier.get(TRACEPARENT_HEADER)


def context_from_traceparent(value: str | None) -> Context:
    if value is None:
        return Context()
    return _PROPAGATOR.extract({TRACEPARENT_HEADER: value})


def is_valid_traceparent(value: str | None) -> bool:
    if value is None:
        return False
    extracted = context_from_traceparent(value)
    span_context = trace.get_current_span(extracted).get_span_context()
    return span_context.is_valid and traceparent_from_context(extracted) is not None


def traceparent_from_headers(headers: object) -> str | None:
    if not isinstance(headers, Mapping):
        return None
    header_items = cast(Mapping[object, object], headers)
    for raw_key, raw_value in header_items.items():
        key = (
            raw_key.decode("ascii", errors="ignore") if isinstance(raw_key, bytes) else str(raw_key)
        )
        if key.lower() != TRACEPARENT_HEADER:
            continue
        value = (
            raw_value.decode("ascii", errors="ignore")
            if isinstance(raw_value, bytes)
            else str(raw_value)
        )
        return value if is_valid_traceparent(value) else None
    return None


def persisted_traceparent(explicit: str | None = None) -> str:
    if explicit is not None:
        if not is_valid_traceparent(explicit):
            raise ValueError("traceparent is invalid")
        return explicit
    current = traceparent_from_context()
    if current is not None:
        return current
    with start_span(
        "messaging.outbox.enqueue",
        kind=SpanKind.PRODUCER,
    ):
        generated = traceparent_from_context()
    if generated is None:
        raise RuntimeError("trace context is unavailable")
    return generated


@contextmanager
def start_span(
    name: str,
    *,
    kind: SpanKind,
    parent_traceparent: str | None = None,
    attributes: Mapping[str, SpanAttribute] | None = None,
) -> Generator[Span]:
    parent_context = context_from_traceparent(parent_traceparent)
    with get_tracer().start_as_current_span(
        name,
        context=parent_context,
        kind=kind,
        attributes=attributes,
        record_exception=False,
        set_status_on_exception=False,
    ) as span:
        yield span


def span_identifiers(span: Span) -> tuple[str, str]:
    context = span.get_span_context()
    return format(context.trace_id, "032x"), format(context.span_id, "016x")
