from collections.abc import Callable, Coroutine, Iterable
from datetime import datetime
from functools import wraps
from typing import Any, Literal, ParamSpec, TypeVar

from prometheus_client import Counter, Gauge, Histogram

from app.modules.messaging.service import OutboxBacklog

QueueLabel = Literal["lanverse.io", "lanverse.media", "unregistered"]
MessageResult = Literal["completed", "duplicate", "rejected", "requeued"]
OutboxPublishResult = Literal["published", "retry_scheduled"]
OutboxState = Literal["pending", "claimed", "manual_attention"]
P = ParamSpec("P")
R = TypeVar("R")

REGISTERED_MESSAGE_EVENT_TYPES = frozenset(
    {
        "script_extraction.requested",
        "episode_planning.requested",
        "script_adaptation.requested",
        "storyboard_draft.requested",
        "generation.requested",
        "media_probe.requested",
        "upload_expiration.requested",
        "upload_cleanup.requested",
        "media_location_migration.requested",
        "media_location_retirement.requested",
    }
)

OUTBOX_PUBLISH_RESULTS = Counter(
    "lanverse_outbox_publish_results_total",
    "Persisted Outbox publish outcomes",
    ("queue", "event_type", "result"),
)
OUTBOX_PUBLISH_DURATION = Histogram(
    "lanverse_outbox_publish_duration_seconds",
    "Outbox publish and persisted outcome duration",
    ("queue", "event_type"),
)
MESSAGE_RESULTS = Counter(
    "lanverse_message_results_total",
    "Worker message processing outcomes",
    ("queue", "event_type", "result"),
)
MESSAGE_HANDLER_DURATION = Histogram(
    "lanverse_message_handler_duration_seconds",
    "Worker message handler duration",
    ("queue", "event_type"),
)
OUTBOX_EVENTS = Gauge(
    "lanverse_outbox_events",
    "Current unpublished Outbox events from PostgreSQL facts",
    ("queue", "state"),
)
OUTBOX_OLDEST_AGE = Gauge(
    "lanverse_outbox_oldest_age_seconds",
    "Age of the oldest unpublished Outbox event from PostgreSQL facts",
    ("queue", "state"),
)
WORKER_INFLIGHT = Gauge(
    "lanverse_worker_inflight",
    "Messages currently handled by this worker process",
    ("queue",),
)
WORKER_CAPACITY = Gauge(
    "lanverse_worker_capacity",
    "Configured RabbitMQ prefetch capacity for this worker process",
    ("queue",),
)

_QUEUE_LABELS: tuple[QueueLabel, ...] = (
    "lanverse.io",
    "lanverse.media",
    "unregistered",
)
_OUTBOX_STATES: tuple[OutboxState, ...] = (
    "pending",
    "claimed",
    "manual_attention",
)


def message_event_type_label(value: str) -> str:
    if value in REGISTERED_MESSAGE_EVENT_TYPES or value == "invalid":
        return value
    return "unregistered"


def message_queue_label(value: str) -> QueueLabel:
    if value == "lanverse.io":
        return "lanverse.io"
    if value == "lanverse.media":
        return "lanverse.media"
    return "unregistered"


def queue_label_for_routing_key(routing_key: str) -> QueueLabel:
    if routing_key.startswith("io."):
        return "lanverse.io"
    if routing_key.startswith("media."):
        return "lanverse.media"
    return "unregistered"


def observe_message_result(
    *,
    queue: str,
    event_type: str,
    result: MessageResult,
    duration_seconds: float,
) -> None:
    queue_label = message_queue_label(queue)
    event_type_label = message_event_type_label(event_type)
    MESSAGE_RESULTS.labels(
        queue=queue_label,
        event_type=event_type_label,
        result=result,
    ).inc()
    MESSAGE_HANDLER_DURATION.labels(
        queue=queue_label,
        event_type=event_type_label,
    ).observe(max(duration_seconds, 0))


def observe_outbox_publish_result(
    *,
    routing_key: str,
    event_type: str,
    result: OutboxPublishResult,
    duration_seconds: float,
) -> None:
    queue = queue_label_for_routing_key(routing_key)
    event_type_label = message_event_type_label(event_type)
    OUTBOX_PUBLISH_RESULTS.labels(
        queue=queue,
        event_type=event_type_label,
        result=result,
    ).inc()
    OUTBOX_PUBLISH_DURATION.labels(
        queue=queue,
        event_type=event_type_label,
    ).observe(max(duration_seconds, 0))


def observe_outbox_backlog(
    backlog: Iterable[OutboxBacklog],
    *,
    observed_at: datetime,
) -> None:
    counts: dict[tuple[QueueLabel, OutboxState], int] = {}
    ages: dict[tuple[QueueLabel, OutboxState], float] = {}
    for item in backlog:
        if item.state not in _OUTBOX_STATES:
            continue
        key = (queue_label_for_routing_key(item.routing_key), item.state)
        counts[key] = counts.get(key, 0) + max(item.count, 0)
        try:
            age = max((observed_at - item.oldest_created_at).total_seconds(), 0)
        except (TypeError, ValueError):
            continue
        ages[key] = max(ages.get(key, 0), age)

    for queue in _QUEUE_LABELS:
        for state in _OUTBOX_STATES:
            key = (queue, state)
            try:
                OUTBOX_EVENTS.labels(queue=queue, state=state).set(counts.get(key, 0))
            except Exception:
                pass
            try:
                OUTBOX_OLDEST_AGE.labels(queue=queue, state=state).set(ages.get(key, 0))
            except Exception:
                pass


def initialize_worker_metrics(*, queue: str, capacity: int) -> None:
    if capacity < 1:
        raise ValueError("worker capacity must be positive")
    queue_label = message_queue_label(queue)
    try:
        WORKER_CAPACITY.labels(queue=queue_label).set(capacity)
    except Exception:
        pass
    try:
        WORKER_INFLIGHT.labels(queue=queue_label).set(0)
    except Exception:
        pass


def track_worker_inflight(
    *,
    queue: str,
    capacity: int,
) -> Callable[
    [Callable[P, Coroutine[Any, Any, R]]],
    Callable[P, Coroutine[Any, Any, R]],
]:
    if capacity < 1:
        raise ValueError("worker capacity must be positive")
    queue_label = message_queue_label(queue)

    def decorator(
        function: Callable[P, Coroutine[Any, Any, R]],
    ) -> Callable[P, Coroutine[Any, Any, R]]:
        @wraps(function)
        async def tracked(*args: P.args, **kwargs: P.kwargs) -> R:
            try:
                WORKER_CAPACITY.labels(queue=queue_label).set(capacity)
            except Exception:
                pass
            incremented = False
            try:
                WORKER_INFLIGHT.labels(queue=queue_label).inc()
                incremented = True
            except Exception:
                pass
            try:
                return await function(*args, **kwargs)
            finally:
                if incremented:
                    try:
                        WORKER_INFLIGHT.labels(queue=queue_label).dec()
                    except Exception:
                        pass

        return tracked

    return decorator
