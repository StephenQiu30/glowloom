from collections.abc import Awaitable, Callable
from datetime import timedelta
from uuid import UUID

import pytest
from opentelemetry import trace
from opentelemetry.sdk.trace.export import SimpleSpanProcessor
from opentelemetry.sdk.trace.export.in_memory_span_exporter import InMemorySpanExporter
from sqlalchemy import select
from sqlalchemy.ext.asyncio import AsyncSession, async_sessionmaker
from uuid6 import uuid7

from app.core.telemetry import configure_telemetry
from app.modules.identity import ActorContext
from app.modules.identity.models import UserAccount, Workspace
from app.modules.messaging import MessageEnvelope
from app.modules.messaging.models import OutboxEvent
from app.modules.messaging.topics import IO_TOPIC
from app.modules.production import ScriptExtractionTaskCommand, create_script_extraction_task
from app.runtime.workers import io as io_worker
from app.runtime.workers.scheduler import publish_outbox_batch


class RecordingPublisher:
    def __init__(self) -> None:
        self.messages: list[tuple[MessageEnvelope, str]] = []

    async def publish(self, envelope: MessageEnvelope, topic: str) -> None:
        self.messages.append((envelope, topic))


class RecordingMessage:
    def __init__(
        self,
        envelope: MessageEnvelope,
        *,
        on_ack: Callable[[], Awaitable[None]] | None = None,
    ) -> None:
        self.body = envelope.model_dump_json().encode("utf-8")
        self.headers = {"traceparent": envelope.traceparent}
        self.on_ack = on_ack
        self.ack_count = 0
        self.nack_requeues: list[bool] = []

    async def ack(self) -> None:
        if self.on_ack is not None:
            await self.on_ack()
        self.ack_count += 1

    async def nack(self, *, requeue: bool) -> None:
        self.nack_requeues.append(requeue)


async def _actor(factory: async_sessionmaker[AsyncSession]) -> ActorContext:
    user_id = uuid7()
    workspace_id = uuid7()
    async with factory() as session:
        async with session.begin():
            session.add_all(
                (
                    UserAccount(
                        id=user_id,
                        email_normalized=f"trace-context-{user_id}@example.com",
                        password_hash="synthetic-not-used",
                        display_name="Trace Context Fixture",
                    ),
                    Workspace(id=workspace_id, name="Trace Context Fixture"),
                )
            )
    return ActorContext(
        user_id=user_id,
        workspace_id=workspace_id,
        membership_id=uuid7(),
        role="owner",
        workspace_status="active",
    )


async def _create_task_inside_server_span(
    factory: async_sessionmaker[AsyncSession],
    actor: ActorContext,
) -> tuple[UUID, trace.SpanContext]:
    tracer = trace.get_tracer("lanverse.tests.trace-context")
    with tracer.start_as_current_span("http.request", kind=trace.SpanKind.SERVER) as span:
        async with factory() as session:
            async with session.begin():
                task = await create_script_extraction_task(
                    session,
                    actor,
                    ScriptExtractionTaskCommand(
                        workspace_id=actor.workspace_id,
                        episode_id=uuid7(),
                        request_id=uuid7(),
                        input_version_id=uuid7(),
                        input_hash="f" * 64,
                        idempotency_key=f"trace-context-{uuid7()}",
                    ),
                    trace_id=str(uuid7()),
                )
        return task.id, span.get_span_context()


@pytest.mark.asyncio
async def test_http_outbox_kafka_and_worker_keep_one_trace_with_distinct_spans(
    session_factory: async_sessionmaker[AsyncSession],
) -> None:
    provider = configure_telemetry(
        service_name="lanverse-trace-test",
        environment="test",
    )
    exporter = InMemorySpanExporter()
    provider.add_span_processor(SimpleSpanProcessor(exporter))
    actor = await _actor(session_factory)

    task_id, server_context = await _create_task_inside_server_span(
        session_factory,
        actor,
    )
    async with session_factory() as session:
        event = await session.scalar(select(OutboxEvent).where(OutboxEvent.aggregate_id == task_id))
    assert event is not None
    assert event.traceparent.split("-")[1] == format(server_context.trace_id, "032x")
    assert event.traceparent.split("-")[2] == format(server_context.span_id, "016x")

    publisher = RecordingPublisher()
    published = await publish_outbox_batch(
        session_factory,
        publisher,
        publisher_id="trace-context-publisher",
        batch_size=10,
        claim_timeout=timedelta(seconds=60),
        topics=frozenset({IO_TOPIC}),
    )
    assert published == 1
    envelope, topic = publisher.messages[0]
    assert topic == "lanverse.io.v1"
    assert envelope.traceparent is not None
    assert envelope.traceparent.split("-")[1] == format(server_context.trace_id, "032x")
    assert envelope.traceparent.split("-")[2] != format(server_context.span_id, "016x")

    message = RecordingMessage(envelope)
    result = await io_worker.process_incoming_message(message, session_factory)
    assert result == "completed"
    assert message.ack_count == 1
    assert message.nack_requeues == []

    duplicate = RecordingMessage(envelope)
    duplicate.headers["traceparent"] = "invalid-message-header"
    duplicate_result = await io_worker.process_incoming_message(
        duplicate,
        session_factory,
    )
    assert duplicate_result == "duplicate"
    assert duplicate.ack_count == 1

    spans = exporter.get_finished_spans()
    producer = next(span for span in spans if span.name == "messaging.outbox.publish")
    consumers = [span for span in spans if span.name == "messaging.message.consume"]
    assert len(consumers) == 2
    assert producer.context is not None
    assert producer.parent is not None
    assert producer.context.trace_id == server_context.trace_id
    assert producer.parent.span_id == server_context.span_id
    for consumer in consumers:
        assert consumer.context is not None
        assert consumer.parent is not None
        assert consumer.context.trace_id == server_context.trace_id
        assert consumer.parent.span_id == producer.context.span_id
        assert consumer.context.span_id != producer.context.span_id
    assert consumers[0].context is not None
    assert consumers[1].context is not None
    assert consumers[0].context.span_id != consumers[1].context.span_id
