import argparse
import asyncio
import logging
import os
import socket
import time
from collections.abc import Callable, Coroutine
from datetime import UTC, datetime, timedelta
from typing import Any, Literal

from opentelemetry.trace import SpanKind, Status, StatusCode
from sqlalchemy.ext.asyncio import AsyncSession, async_sessionmaker

from app.core.config import Settings, get_settings
from app.core.database import session_factory
from app.core.logging import configure_logging, log_event
from app.core.schema import assert_database_schema
from app.core.telemetry import (
    configure_telemetry,
    span_identifiers,
    start_span,
    traceparent_from_context,
)
from app.integrations.kafka import KafkaPublisher
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
    topic_label,
)
from app.modules.messaging.topics import REGISTERED_TOPICS
from app.modules.scheduling.dispatcher import dispatch_due_schedules
from app.runtime.model_registry import register_implemented_models

LOGGER = logging.getLogger("lanverse.outbox")

SchedulerMode = Literal["all", "schedule", "outbox"]
SchedulerService = Callable[[Settings], Coroutine[Any, Any, None]]


async def publish_outbox_batch(
    factory: async_sessionmaker[AsyncSession],
    publisher: MessagePublisher,
    *,
    publisher_id: str,
    batch_size: int,
    claim_timeout: timedelta,
    topics: frozenset[str],
) -> int:
    try:
        return await _publish_outbox_batch(
            factory,
            publisher,
            publisher_id=publisher_id,
            batch_size=batch_size,
            claim_timeout=claim_timeout,
            topics=topics,
        )
    finally:
        await refresh_outbox_backlog_metrics(factory, topics=topics)


async def refresh_outbox_backlog_metrics(
    factory: async_sessionmaker[AsyncSession],
    *,
    topics: frozenset[str],
) -> None:
    try:
        async with factory() as session:
            backlog = await outbox_backlog(session, topics=topics)
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
    topics: frozenset[str],
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
                topics=topics,
            )

    published = 0
    for event in events:
        started = time.perf_counter()
        event_type_label = message_event_type_label(event.event_type)
        topic = topic_label(event.topic)
        with start_span(
            "messaging.outbox.publish",
            kind=SpanKind.PRODUCER,
            parent_traceparent=event.traceparent,
            attributes={
                "messaging.system": "kafka",
                "messaging.operation": "publish",
                "messaging.event.type": event_type_label,
                "messaging.destination.name": topic,
            },
        ) as span:
            trace_id, span_id = span_identifiers(span)
            try:
                await publisher.publish(
                    envelope_from_event(
                        event,
                        traceparent=traceparent_from_context(),
                    ),
                    event.topic,
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
                    topic=event.topic,
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
                    topic=topic,
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
                    topic=event.topic,
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
                    topic=topic,
                    attempt=event.attempt_count,
                    result="published",
                    duration_ms=round(duration_seconds * 1000, 2),
                )
                published += 1
    return published


async def _prepare_runtime(settings: Settings, *, service_name: str) -> None:
    configure_logging(
        settings.log_level,
        service=service_name,
        environment=settings.environment,
    )
    configure_telemetry(
        service_name=service_name,
        environment=settings.environment,
    )
    register_implemented_models()
    await assert_database_schema()


async def run_schedule_dispatcher(settings: Settings) -> None:
    await _prepare_runtime(settings, service_name="lanverse-schedule-dispatcher")
    dispatcher_id = f"{socket.gethostname()}:{os.getpid()}:schedule"
    while True:
        dispatched = await dispatch_due_schedules(
            session_factory,
            dispatcher_id=dispatcher_id,
            now=None,
            batch_size=settings.outbox_batch_size,
            lease_duration=timedelta(seconds=settings.outbox_claim_seconds),
        )
        if dispatched == 0:
            await asyncio.sleep(settings.outbox_poll_seconds)


async def run_outbox_publisher(settings: Settings) -> None:
    topics = parse_outbox_topics(settings.outbox_topics)
    await _prepare_runtime(settings, service_name="lanverse-outbox-publisher")
    publisher = KafkaPublisher(settings.kafka_bootstrap_servers)
    publisher_id = f"{socket.gethostname()}:{os.getpid()}:outbox"
    await publisher.connect()
    try:
        while True:
            published = await publish_outbox_batch(
                session_factory,
                publisher,
                publisher_id=publisher_id,
                batch_size=settings.outbox_batch_size,
                claim_timeout=timedelta(seconds=settings.outbox_claim_seconds),
                topics=topics,
            )
            if published == 0:
                await asyncio.sleep(settings.outbox_poll_seconds)
    finally:
        await publisher.close()


async def run_scheduler(settings: Settings) -> None:
    async with asyncio.TaskGroup() as tasks:
        tasks.create_task(run_schedule_dispatcher(settings))
        tasks.create_task(run_outbox_publisher(settings))


def parse_outbox_topics(value: str) -> frozenset[str]:
    topics = frozenset(topic.strip() for topic in value.split(",") if topic.strip())
    if not topics:
        raise ValueError("OUTBOX_TOPICS must not be empty")
    unknown_topics = topics - REGISTERED_TOPICS
    if unknown_topics:
        names = ", ".join(sorted(unknown_topics))
        raise ValueError(f"OUTBOX_TOPICS contains topics that are not registered: {names}")
    return topics


def scheduler_service(mode: SchedulerMode) -> SchedulerService:
    services: dict[SchedulerMode, SchedulerService] = {
        "all": run_scheduler,
        "schedule": run_schedule_dispatcher,
        "outbox": run_outbox_publisher,
    }
    return services[mode]


def main() -> None:
    parser = argparse.ArgumentParser(description="Run a Lanverse scheduling process")
    parser.add_argument(
        "mode",
        choices=("all", "schedule", "outbox"),
        default="all",
        nargs="?",
    )
    arguments = parser.parse_args()
    asyncio.run(scheduler_service(arguments.mode)(get_settings()))


if __name__ == "__main__":
    main()
