import asyncio
import logging
import time
from typing import Literal, Protocol

import aio_pika
from opentelemetry.trace import SpanKind, Status, StatusCode
from pydantic import ValidationError
from sqlalchemy.ext.asyncio import AsyncSession, async_sessionmaker

from app.core.config import Settings, get_settings
from app.core.database import session_factory
from app.core.logging import configure_logging, log_event
from app.core.telemetry import (
    configure_telemetry,
    span_identifiers,
    start_span,
    traceparent_from_headers,
)
from app.integrations.ffprobe import FfprobeMediaProbe
from app.integrations.minio import MinioObjectStorage
from app.integrations.rabbitmq import declare_task_topology
from app.model_registry import register_implemented_models
from app.modules.media import MediaProbePort
from app.modules.media.cleanup_consumer import consume_upload_cleanup
from app.modules.media.consumer import consume_media_probe
from app.modules.media.expiration_consumer import consume_upload_expiration
from app.modules.media.location_consumer import consume_media_location_migration
from app.modules.media.retirement_consumer import consume_media_location_retirement
from app.modules.media.storage import ObjectStoragePort
from app.modules.messaging import MessageEnvelope
from app.modules.messaging.metrics import (
    message_event_type_label,
    observe_message_result,
)

MEDIA_WORKER_MAX_IN_FLIGHT = 2
MAX_MESSAGE_BYTES = 64 * 1024
WorkerResult = Literal["completed", "duplicate", "rejected", "requeued"]
QUEUE_NAME = "lanverse.media"
LOGGER = logging.getLogger("lanverse.worker")


class IncomingMessage(Protocol):
    body: bytes

    async def ack(self) -> None: ...

    async def nack(self, *, requeue: bool) -> None: ...


async def process_incoming_message(
    message: IncomingMessage,
    factory: async_sessionmaker[AsyncSession],
    *,
    storage: ObjectStoragePort,
    probe: MediaProbePort,
    cleanup_batch_size: int = 100,
    storage_profile: str = "default",
    storage_bucket: str = "lanverse-media",
    location_rollback_seconds: int = 86400,
) -> WorkerResult:
    started = time.perf_counter()
    if len(message.body) > MAX_MESSAGE_BYTES:
        await message.ack()
        _observe_invalid_message(started, error_type="MessageTooLarge")
        return "rejected"
    try:
        envelope = MessageEnvelope.model_validate_json(message.body)
    except ValidationError:
        await message.ack()
        _observe_invalid_message(started, error_type="MessageValidationError")
        return "rejected"
    event_type_label = message_event_type_label(envelope.event_type)
    parent_traceparent = (
        traceparent_from_headers(getattr(message, "headers", None)) or envelope.traceparent
    )
    with start_span(
        "messaging.message.consume",
        kind=SpanKind.CONSUMER,
        parent_traceparent=parent_traceparent,
        attributes={
            "messaging.system": "rabbitmq",
            "messaging.operation": "process",
            "messaging.event.type": event_type_label,
            "messaging.destination.name": QUEUE_NAME,
        },
    ) as span:
        result = await _process_valid_envelope(
            message,
            envelope,
            factory,
            storage=storage,
            probe=probe,
            cleanup_batch_size=cleanup_batch_size,
            storage_profile=storage_profile,
            storage_bucket=storage_bucket,
            location_rollback_seconds=location_rollback_seconds,
        )
        span.set_attribute("messaging.operation.result", result)
        if result == "requeued":
            span.set_status(Status(StatusCode.ERROR))
        duration_seconds = time.perf_counter() - started
        trace_id, span_id = span_identifiers(span)
        observe_message_result(
            queue=QUEUE_NAME,
            event_type=envelope.event_type,
            result=result,
            duration_seconds=duration_seconds,
        )
        failed = result in {"rejected", "requeued"}
        common_attributes: dict[str, object] = {
            "request_id": envelope.trace_id,
            "trace_id": trace_id,
            "span_id": span_id,
            "event_id": str(envelope.event_id),
            "event_type": event_type_label,
            "queue": QUEUE_NAME,
            "result": result,
            "duration_ms": round(duration_seconds * 1000, 2),
        }
        if failed:
            log_event(
                LOGGER,
                logging.WARNING,
                "message.consume.failed",
                "message consume failed",
                **common_attributes,
                retryable=result == "requeued",
                error_type=(
                    "MessageProcessingError" if result == "requeued" else "MessageRejected"
                ),
            )
        else:
            log_event(
                LOGGER,
                logging.INFO,
                "message.consume.completed",
                "message consume completed",
                **common_attributes,
            )
        return result


def _observe_invalid_message(started: float, *, error_type: str) -> None:
    duration_seconds = time.perf_counter() - started
    observe_message_result(
        queue=QUEUE_NAME,
        event_type="invalid",
        result="rejected",
        duration_seconds=duration_seconds,
    )
    log_event(
        LOGGER,
        logging.WARNING,
        "message.consume.failed",
        "message consume rejected",
        event_type="invalid",
        queue=QUEUE_NAME,
        result="rejected",
        duration_ms=round(duration_seconds * 1000, 2),
        retryable=False,
        error_type=error_type,
    )


async def _process_valid_envelope(
    message: IncomingMessage,
    envelope: MessageEnvelope,
    factory: async_sessionmaker[AsyncSession],
    *,
    storage: ObjectStoragePort,
    probe: MediaProbePort,
    cleanup_batch_size: int,
    storage_profile: str,
    storage_bucket: str,
    location_rollback_seconds: int,
) -> WorkerResult:
    try:
        async with factory() as session:
            async with session.begin():
                if envelope.event_type == "upload_expiration.requested":
                    result = await consume_upload_expiration(
                        session,
                        envelope,
                        storage=storage,
                    )
                elif envelope.event_type == "upload_cleanup.requested":
                    result = await consume_upload_cleanup(
                        session,
                        envelope,
                        storage=storage,
                        batch_size=cleanup_batch_size,
                    )
                elif envelope.event_type == "media_location_migration.requested":
                    result = await consume_media_location_migration(
                        session,
                        envelope,
                        storage=storage,
                        storage_profile=storage_profile,
                        storage_bucket=storage_bucket,
                        rollback_seconds=location_rollback_seconds,
                    )
                elif envelope.event_type == "media_location_retirement.requested":
                    result = await consume_media_location_retirement(
                        session,
                        envelope,
                        storage=storage,
                        storage_profile=storage_profile,
                        storage_bucket=storage_bucket,
                    )
                else:
                    result = await consume_media_probe(
                        session,
                        envelope,
                        storage=storage,
                        probe=probe,
                    )
    except Exception:
        await message.nack(requeue=True)
        return "requeued"
    await message.ack()
    return result


async def run_media_worker(settings: Settings) -> None:
    configure_logging(
        settings.log_level,
        service="lanverse-media-worker",
        environment=settings.environment,
    )
    configure_telemetry(
        service_name="lanverse-media-worker",
        environment=settings.environment,
    )
    register_implemented_models()
    storage = MinioObjectStorage(
        settings.minio_endpoint,
        settings.minio_access_key,
        settings.minio_secret_key,
        settings.minio_bucket,
        secure=settings.minio_secure,
        thread_limit=settings.storage_thread_limit,
        operation_timeout_seconds=settings.storage_operation_timeout_seconds,
    )
    probe = FfprobeMediaProbe(timeout_seconds=settings.media_probe_timeout_seconds)
    connection = await aio_pika.connect_robust(settings.rabbitmq_url, timeout=3)
    try:
        channel = await connection.channel()
        await channel.set_qos(prefetch_count=MEDIA_WORKER_MAX_IN_FLIGHT)
        _, _, media_queue = await declare_task_topology(channel)

        async def on_message(message: aio_pika.abc.AbstractIncomingMessage) -> None:
            await process_incoming_message(
                message,
                session_factory,
                storage=storage,
                probe=probe,
                cleanup_batch_size=settings.media_cleanup_batch_size,
                storage_profile="default",
                storage_bucket=settings.minio_bucket,
                location_rollback_seconds=settings.media_location_rollback_seconds,
            )

        await media_queue.consume(on_message, no_ack=False)
        await asyncio.Future()
    finally:
        await connection.close()


def main() -> None:
    asyncio.run(run_media_worker(get_settings()))


if __name__ == "__main__":
    main()
