import asyncio
import logging
import os
import socket
import time
from datetime import UTC, datetime, timedelta

from opentelemetry.trace import SpanKind, Status, StatusCode
from sqlalchemy.ext.asyncio import AsyncSession, async_sessionmaker

from app.core.config import Settings, get_settings
from app.core.database import session_factory
from app.core.logging import configure_logging, log_event
from app.core.migrations import assert_database_at_head
from app.core.telemetry import (
    configure_telemetry,
    span_identifiers,
    start_span,
    traceparent_from_context,
)
from app.integrations.rabbitmq import RabbitMQPublisher
from app.model_registry import register_implemented_models
from app.modules.messaging import (
    MessagePublisher,
    claim_outbox_events,
    envelope_from_event,
    mark_outbox_published,
    outbox_backlog,
    release_outbox_for_retry,
)
from app.modules.messaging.metrics import (
    message_event_type_label,
    observe_outbox_backlog,
    observe_outbox_publish_result,
    queue_label_for_routing_key,
)
from app.modules.scheduling.dispatcher import dispatch_due_schedules

LOGGER = logging.getLogger("lanverse.outbox")


async def publish_outbox_batch(
    factory: async_sessionmaker[AsyncSession],
    publisher: MessagePublisher,
    *,
    publisher_id: str,
    batch_size: int,
    claim_timeout: timedelta,
) -> int:
    try:
        return await _publish_outbox_batch(
            factory,
            publisher,
            publisher_id=publisher_id,
            batch_size=batch_size,
            claim_timeout=claim_timeout,
        )
    finally:
        await refresh_outbox_backlog_metrics(factory)


async def refresh_outbox_backlog_metrics(
    factory: async_sessionmaker[AsyncSession],
) -> None:
    try:
        async with factory() as session:
            backlog = await outbox_backlog(session)
        observe_outbox_backlog(backlog, observed_at=datetime.now(UTC))
    except Exception:
        pass


async def _publish_outbox_batch(
    factory: async_sessionmaker[AsyncSession],
    publisher: MessagePublisher,
    *,
    publisher_id: str,
    batch_size: int,
    claim_timeout: timedelta,
) -> int:
    claimed_at = datetime.now(UTC)
    async with factory() as session:
        async with session.begin():
            events = await claim_outbox_events(
                session,
                publisher_id=publisher_id,
                now=claimed_at,
                batch_size=batch_size,
                claim_timeout=claim_timeout,
            )

    published = 0
    for event in events:
        started = time.perf_counter()
        event_type_label = message_event_type_label(event.event_type)
        queue = queue_label_for_routing_key(event.routing_key)
        with start_span(
            "messaging.outbox.publish",
            kind=SpanKind.PRODUCER,
            parent_traceparent=event.traceparent,
            attributes={
                "messaging.system": "rabbitmq",
                "messaging.operation": "publish",
                "messaging.event.type": event_type_label,
                "messaging.destination.name": queue,
                "messaging.routing_key": event.routing_key,
            },
        ) as span:
            trace_id, span_id = span_identifiers(span)
            try:
                await publisher.publish(
                    envelope_from_event(
                        event,
                        traceparent=traceparent_from_context(),
                    ),
                    event.routing_key,
                )
            except Exception as error:
                span.set_attribute("error.type", type(error).__name__)
                span.set_status(Status(StatusCode.ERROR))
                async with factory() as session:
                    async with session.begin():
                        await release_outbox_for_retry(
                            session,
                            event.id,
                            publisher_id=publisher_id,
                            now=datetime.now(UTC),
                            error=error,
                        )
                duration_seconds = time.perf_counter() - started
                observe_outbox_publish_result(
                    routing_key=event.routing_key,
                    event_type=event.event_type,
                    result="retry_scheduled",
                    duration_seconds=duration_seconds,
                )
                log_event(
                    LOGGER,
                    logging.WARNING,
                    "outbox.publish.failed",
                    "outbox publish retry scheduled",
                    request_id=event.trace_id,
                    trace_id=trace_id,
                    span_id=span_id,
                    event_id=str(event.id),
                    event_type=event_type_label,
                    queue=queue,
                    attempt=event.attempt_count,
                    result="retry_scheduled",
                    duration_ms=round(duration_seconds * 1000, 2),
                    retryable=True,
                    error_type=type(error).__name__,
                )
            else:
                async with factory() as session:
                    async with session.begin():
                        await mark_outbox_published(
                            session,
                            event.id,
                            publisher_id=publisher_id,
                            now=datetime.now(UTC),
                        )
                duration_seconds = time.perf_counter() - started
                observe_outbox_publish_result(
                    routing_key=event.routing_key,
                    event_type=event.event_type,
                    result="published",
                    duration_seconds=duration_seconds,
                )
                log_event(
                    LOGGER,
                    logging.INFO,
                    "outbox.publish.completed",
                    "outbox publish completed",
                    request_id=event.trace_id,
                    trace_id=trace_id,
                    span_id=span_id,
                    event_id=str(event.id),
                    event_type=event_type_label,
                    queue=queue,
                    attempt=event.attempt_count,
                    result="published",
                    duration_ms=round(duration_seconds * 1000, 2),
                )
                published += 1
    return published


async def run_scheduler(settings: Settings) -> None:
    configure_logging(
        settings.log_level,
        service="lanverse-scheduler",
        environment=settings.environment,
    )
    configure_telemetry(
        service_name="lanverse-scheduler",
        environment=settings.environment,
    )
    register_implemented_models()
    await assert_database_at_head()
    publisher = RabbitMQPublisher(settings.rabbitmq_url)
    publisher_id = f"{socket.gethostname()}:{os.getpid()}"
    await publisher.connect()
    try:
        while True:
            dispatched = await dispatch_due_schedules(
                session_factory,
                dispatcher_id=publisher_id,
                now=None,
                batch_size=settings.outbox_batch_size,
                lease_duration=timedelta(seconds=settings.outbox_claim_seconds),
            )
            published = await publish_outbox_batch(
                session_factory,
                publisher,
                publisher_id=publisher_id,
                batch_size=settings.outbox_batch_size,
                claim_timeout=timedelta(seconds=settings.outbox_claim_seconds),
            )
            if dispatched == 0 and published == 0:
                await asyncio.sleep(settings.outbox_poll_seconds)
    finally:
        await publisher.close()


def main() -> None:
    asyncio.run(run_scheduler(get_settings()))


if __name__ == "__main__":
    main()
