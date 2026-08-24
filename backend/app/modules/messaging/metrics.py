from collections.abc import Callable, Coroutine, Iterable
from datetime import datetime
from functools import wraps
from typing import Any, Literal, ParamSpec, TypeVar

from prometheus_client import Counter, Gauge, Histogram

from app.modules.messaging.service import OutboxBacklog

TopicLabel = Literal["lanverse.io.v1", "lanverse.media.v1", "unregistered"]
MessageResult = Literal["completed", "duplicate", "rejected", "requeued"]
OutboxPublishResult = Literal["published", "retry_scheduled"]
OutboxState = Literal["pending", "claimed", "manual_attention"]
P = ParamSpec("P")
R = TypeVar("R")

REGISTERED_MESSAGE_EVENT_TYPES = frozenset(
    {
        "script_extraction.requested",
        "episode_planning.requested",
        "production_bible.requested",
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
    ("topic", "event_type", "result"),
)
OUTBOX_PUBLISH_DURATION = Histogram(
    "lanverse_outbox_publish_duration_seconds",
    "Outbox publish and persisted outcome duration",
    ("topic", "event_type"),
)
MESSAGE_RESULTS = Counter(
    "lanverse_message_results_total",
    "Worker message processing outcomes",
    ("topic", "event_type", "result"),
)
MESSAGE_HANDLER_DURATION = Histogram(
    "lanverse_message_handler_duration_seconds",
    "Worker message handler duration",
    ("topic", "event_type"),
)
OUTBOX_EVENTS = Gauge(
    "lanverse_outbox_events",
    "Current unpublished Outbox events from PostgreSQL facts",
    ("topic", "state"),
)
OUTBOX_OLDEST_AGE = Gauge(
    "lanverse_outbox_oldest_age_seconds",
    "Age of the oldest unpublished Outbox event from PostgreSQL facts",
    ("topic", "state"),
)
WORKER_INFLIGHT = Gauge(
    "lanverse_worker_inflight",
    "Messages currently handled by this worker process",
    ("topic",),
)
WORKER_CAPACITY = Gauge(
    "lanverse_worker_capacity",
    "Configured Kafka message processing capacity for this worker process",
    ("topic",),
)

_TOPIC_LABELS: tuple[TopicLabel, ...] = (
    "lanverse.io.v1",
    "lanverse.media.v1",
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


def topic_label(value: str) -> TopicLabel:
    if value == "lanverse.io.v1":
        return "lanverse.io.v1"
    if value == "lanverse.media.v1":
        return "lanverse.media.v1"
    return "unregistered"


def observe_message_result(
    *,
    topic: str,
    event_type: str,
    result: MessageResult,
    duration_seconds: float,
) -> None:
    destination = topic_label(topic)
    event_type_label = message_event_type_label(event_type)
    MESSAGE_RESULTS.labels(
        topic=destination,
        event_type=event_type_label,
        result=result,
    ).inc()
    MESSAGE_HANDLER_DURATION.labels(
        topic=destination,
        event_type=event_type_label,
    ).observe(max(duration_seconds, 0))


def observe_outbox_publish_result(
    *,
    topic: str,
    event_type: str,
    result: OutboxPublishResult,
    duration_seconds: float,
) -> None:
    destination = topic_label(topic)
    event_type_label = message_event_type_label(event_type)
    OUTBOX_PUBLISH_RESULTS.labels(
        topic=destination,
        event_type=event_type_label,
        result=result,
    ).inc()
    OUTBOX_PUBLISH_DURATION.labels(
        topic=destination,
        event_type=event_type_label,
    ).observe(max(duration_seconds, 0))


def observe_outbox_backlog(
    backlog: Iterable[OutboxBacklog],
    *,
    observed_at: datetime,
) -> None:
    counts: dict[tuple[TopicLabel, OutboxState], int] = {}
    ages: dict[tuple[TopicLabel, OutboxState], float] = {}
    for item in backlog:
        if item.state not in _OUTBOX_STATES:
            continue
        key = (topic_label(item.topic), item.state)
        counts[key] = counts.get(key, 0) + max(item.count, 0)
        try:
            age = max((observed_at - item.oldest_created_at).total_seconds(), 0)
        except (TypeError, ValueError):
            continue
        ages[key] = max(ages.get(key, 0), age)

    for topic in _TOPIC_LABELS:
        for state in _OUTBOX_STATES:
            key = (topic, state)
            try:
                OUTBOX_EVENTS.labels(topic=topic, state=state).set(counts.get(key, 0))
            except Exception:
                pass
            try:
                OUTBOX_OLDEST_AGE.labels(topic=topic, state=state).set(ages.get(key, 0))
            except Exception:
                pass


def initialize_worker_metrics(*, topic: str, capacity: int) -> None:
    if capacity < 1:
        raise ValueError("worker capacity must be positive")
    destination = topic_label(topic)
    try:
        WORKER_CAPACITY.labels(topic=destination).set(capacity)
    except Exception:
        pass
    try:
        WORKER_INFLIGHT.labels(topic=destination).set(0)
    except Exception:
        pass


def track_worker_inflight(
    *,
    topic: str,
    capacity: int,
) -> Callable[
    [Callable[P, Coroutine[Any, Any, R]]],
    Callable[P, Coroutine[Any, Any, R]],
]:
    if capacity < 1:
        raise ValueError("worker capacity must be positive")
    destination = topic_label(topic)

    def decorator(
        function: Callable[P, Coroutine[Any, Any, R]],
    ) -> Callable[P, Coroutine[Any, Any, R]]:
        @wraps(function)
        async def tracked(*args: P.args, **kwargs: P.kwargs) -> R:
            try:
                WORKER_CAPACITY.labels(topic=destination).set(capacity)
            except Exception:
                pass
            incremented = False
            try:
                WORKER_INFLIGHT.labels(topic=destination).inc()
                incremented = True
            except Exception:
                pass
            try:
                return await function(*args, **kwargs)
            finally:
                if incremented:
                    try:
                        WORKER_INFLIGHT.labels(topic=destination).dec()
                    except Exception:
                        pass

        return tracked

    return decorator
