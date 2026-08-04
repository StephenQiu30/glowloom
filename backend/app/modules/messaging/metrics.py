from typing import Literal

from prometheus_client import Counter, Histogram

QueueLabel = Literal["lanverse.io", "lanverse.media", "unregistered"]
MessageResult = Literal["completed", "duplicate", "rejected", "requeued"]
OutboxPublishResult = Literal["published", "retry_scheduled"]

REGISTERED_MESSAGE_EVENT_TYPES = frozenset(
    {
        "script_extraction.requested",
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
