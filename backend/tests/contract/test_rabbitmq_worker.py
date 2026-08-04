import asyncio
import os
from datetime import UTC, datetime
from uuid import UUID

import aio_pika
import httpx
import pytest
from sqlalchemy import func, select
from sqlalchemy.ext.asyncio import AsyncSession, async_sessionmaker
from uuid6 import uuid7

from app.core.database import Base, create_engine, validate_test_database_url
from app.integrations.rabbitmq import IO_QUEUE, MEDIA_QUEUE, RabbitMQPublisher
from app.io_worker import IO_WORKER_MAX_IN_FLIGHT, process_incoming_message
from app.media_worker import MEDIA_WORKER_MAX_IN_FLIGHT
from app.modules.identity import ActorContext
from app.modules.identity.models import UserAccount, Workspace
from app.modules.messaging import MessageEnvelope, envelope_from_event
from app.modules.messaging.models import InboxDelivery, OutboxEvent
from app.modules.production import ScriptExtractionTaskCommand, create_script_extraction_task
from app.modules.production.models import CostEntry, GenerationAttempt, Reservation, Task
from tests.integration.test_generation_task_cancellation import submit_queued_generation
from tests.support.external_contracts import rabbitmq_contract_url


def _capacity_envelope(trace_id: str) -> MessageEnvelope:
    task_id = uuid7()
    return MessageEnvelope(
        event_id=uuid7(),
        event_type="media_probe.requested",
        schema_version=1,
        aggregate_id=task_id,
        workspace_id=uuid7(),
        occurred_at=datetime.now(UTC),
        trace_id=trace_id,
        traceparent="00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01",
        causation_event_id=None,
        payload={"task_id": str(task_id)},
    )


@pytest.mark.skipif(
    os.getenv("LANVERSE_RUN_RABBITMQ_CONTRACT") != "1",
    reason="set LANVERSE_RUN_RABBITMQ_CONTRACT=1 with an isolated RabbitMQ vhost",
)
@pytest.mark.asyncio
async def test_worker_commits_inbox_before_manual_ack_on_real_queue() -> None:
    rabbitmq_url = rabbitmq_contract_url()
    test_database_url = validate_test_database_url(
        os.getenv(
            "TEST_DATABASE_URL",
            "postgresql+asyncpg://postgres@127.0.0.1:5432/lanverse_test",
        ),
        "postgresql+asyncpg://postgres@127.0.0.1:5432/lanverse",
    )
    publisher = RabbitMQPublisher(rabbitmq_url)
    observer = await aio_pika.connect_robust(rabbitmq_url, timeout=3)
    engine = create_engine(test_database_url)
    message: aio_pika.abc.AbstractIncomingMessage | None = None
    try:
        await publisher.connect()
        channel = await observer.channel()
        queue = await channel.declare_queue(IO_QUEUE, durable=True)
        if queue.declaration_result.message_count != 0:
            pytest.skip("lanverse.io is not empty; refusing to consume existing messages")

        async with engine.begin() as connection:
            await connection.run_sync(Base.metadata.drop_all)
            await connection.run_sync(Base.metadata.create_all)
        factory = async_sessionmaker(engine, expire_on_commit=False)
        user_id = uuid7()
        workspace_id = uuid7()
        actor = ActorContext(
            user_id=user_id,
            workspace_id=workspace_id,
            membership_id=uuid7(),
            role="owner",
            workspace_status="active",
        )
        async with factory() as session:
            async with session.begin():
                session.add_all(
                    (
                        UserAccount(
                            id=user_id,
                            email_normalized=f"rabbit-worker-{user_id}@example.com",
                            password_hash="synthetic-not-used",
                            display_name="Rabbit Worker Fixture",
                        ),
                        Workspace(id=workspace_id, name="Rabbit Worker Fixture"),
                    )
                )
                task = await create_script_extraction_task(
                    session,
                    actor,
                    ScriptExtractionTaskCommand(
                        workspace_id=workspace_id,
                        episode_id=uuid7(),
                        request_id=uuid7(),
                        input_version_id=uuid7(),
                        input_hash="d" * 64,
                        idempotency_key=f"rabbit-worker-{uuid7()}",
                    ),
                    trace_id="rabbit-worker-contract-trace",
                )
                task_id = task.id
            event = await session.scalar(
                select(OutboxEvent).where(OutboxEvent.aggregate_id == task_id)
            )
            assert event is not None
            envelope = envelope_from_event(event)

        await publisher.publish(envelope, "io.script.extract")
        message = await queue.get(timeout=3, fail=False)
        assert message is not None
        assert message.message_id == str(envelope.event_id)

        result = await process_incoming_message(message, factory)
        assert result == "completed"
        assert message.processed is True

        queue_state = await channel.declare_queue(IO_QUEUE, durable=True, passive=True)
        assert queue_state.declaration_result.message_count == 0
        async with factory() as session:
            task = await session.get(Task, task_id)
            inbox = await session.scalar(select(InboxDelivery))
            assert task is not None
            assert task.status == "failed"
            assert task.error_code == "ai_service_unavailable"
            assert task.revision == 2
            assert inbox is not None
            assert inbox.status == "completed"
            assert inbox.event_id == envelope.event_id

        async with engine.begin() as connection:
            await connection.run_sync(Base.metadata.drop_all)
        await channel.close()
    finally:
        if message is not None and not message.processed:
            await message.nack(requeue=True)
        await publisher.close()
        await observer.close()
        await engine.dispose()


@pytest.mark.skipif(
    os.getenv("LANVERSE_RUN_RABBITMQ_CONTRACT") != "1",
    reason="set LANVERSE_RUN_RABBITMQ_CONTRACT=1 with an isolated RabbitMQ vhost",
)
@pytest.mark.asyncio
async def test_real_generation_message_persists_attempt_and_releases_without_provider(
    client: httpx.AsyncClient,
    session_factory: async_sessionmaker[AsyncSession],
) -> None:
    rabbitmq_url = rabbitmq_contract_url()
    publisher = RabbitMQPublisher(rabbitmq_url)
    observer = await aio_pika.connect_robust(rabbitmq_url, timeout=3)
    message: aio_pika.abc.AbstractIncomingMessage | None = None
    try:
        await publisher.connect()
        channel = await observer.channel()
        queue = await channel.declare_queue(IO_QUEUE, durable=True)
        if queue.declaration_result.message_count != 0:
            pytest.skip("lanverse.io is not empty; refusing to consume existing messages")

        _, _, submitted = await submit_queued_generation(
            client,
            session_factory,
            submission_key="real-rabbit-generation-attempt",
        )
        task_id = UUID(submitted["task"]["id"])
        reservation_id = UUID(submitted["reservation"]["id"])
        async with session_factory() as session:
            event = await session.scalar(
                select(OutboxEvent).where(
                    OutboxEvent.aggregate_id == task_id,
                    OutboxEvent.event_type == "generation.requested",
                )
            )
            assert event is not None
            envelope = envelope_from_event(event)

        await publisher.publish(envelope, "io.provider.submit")
        message = await queue.get(timeout=3, fail=False)
        assert message is not None
        assert message.message_id == str(envelope.event_id)

        assert await process_incoming_message(message, session_factory) == "completed"
        assert message.processed is True
        queue_state = await channel.declare_queue(IO_QUEUE, durable=True, passive=True)
        assert queue_state.declaration_result.message_count == 0

        async with session_factory() as session:
            task = await session.get(Task, task_id)
            attempt = await session.scalar(
                select(GenerationAttempt).where(GenerationAttempt.task_id == task_id)
            )
            reservation = await session.get(
                Reservation,
                reservation_id,
            )
            release = await session.scalar(
                select(CostEntry).where(
                    CostEntry.reservation_id == reservation_id,
                    CostEntry.entry_type == "release",
                )
            )
            inbox = await session.scalar(
                select(InboxDelivery).where(InboxDelivery.task_id == task_id)
            )
            assert task is not None and task.status == "failed"
            assert attempt is not None and attempt.status == "failed"
            assert reservation is not None and reservation.status == "released"
            assert release is not None and release.attempt_id == attempt.id
            assert inbox is not None and inbox.status == "completed"
        await channel.close()
    finally:
        if message is not None and not message.processed:
            await message.nack(requeue=True)
        await publisher.close()
        await observer.close()


@pytest.mark.skipif(
    os.getenv("LANVERSE_RUN_RABBITMQ_CONTRACT") != "1",
    reason="set LANVERSE_RUN_RABBITMQ_CONTRACT=1 with an isolated RabbitMQ vhost",
)
@pytest.mark.asyncio
async def test_real_generation_protocol_error_acks_after_manual_attention_release(
    client: httpx.AsyncClient,
    session_factory: async_sessionmaker[AsyncSession],
) -> None:
    rabbitmq_url = rabbitmq_contract_url()
    publisher = RabbitMQPublisher(rabbitmq_url)
    observer = await aio_pika.connect_robust(rabbitmq_url, timeout=3)
    received: list[aio_pika.abc.AbstractIncomingMessage] = []
    try:
        await publisher.connect()
        channel = await observer.channel()
        queue = await channel.declare_queue(IO_QUEUE, durable=True)
        if queue.declaration_result.message_count != 0:
            pytest.skip("lanverse.io is not empty; refusing to consume existing messages")

        _, _, submitted = await submit_queued_generation(
            client,
            session_factory,
            submission_key="real-rabbit-generation-protocol-error",
        )
        task_id = UUID(submitted["task"]["id"])
        reservation_id = UUID(submitted["reservation"]["id"])
        async with session_factory() as session:
            event = await session.scalar(
                select(OutboxEvent).where(
                    OutboxEvent.aggregate_id == task_id,
                    OutboxEvent.event_type == "generation.requested",
                )
            )
            assert event is not None
            envelope = envelope_from_event(event).model_copy(
                update={"schema_version": 99}
            )

        await publisher.publish(envelope, "io.provider.submit")
        await publisher.publish(envelope, "io.provider.submit")
        first = await queue.get(timeout=3, fail=False)
        second = await queue.get(timeout=3, fail=False)
        assert first is not None and second is not None
        received.extend((first, second))

        assert await process_incoming_message(first, session_factory) == "rejected"
        assert await process_incoming_message(second, session_factory) == "duplicate"
        assert first.processed is True and second.processed is True
        queue_state = await channel.declare_queue(IO_QUEUE, durable=True, passive=True)
        assert queue_state.declaration_result.message_count == 0

        async with session_factory() as session:
            task = await session.get(Task, task_id)
            reservation = await session.get(Reservation, reservation_id)
            release = await session.scalar(
                select(CostEntry).where(
                    CostEntry.reservation_id == reservation_id,
                    CostEntry.entry_type == "release",
                )
            )
            attempt_count = await session.scalar(
                select(func.count())
                .select_from(GenerationAttempt)
                .where(GenerationAttempt.task_id == task_id)
            )
            inbox = await session.scalar(
                select(InboxDelivery).where(InboxDelivery.event_id == envelope.event_id)
            )
            assert task is not None and task.status == "failed"
            assert task.progress_stage == "manual_attention"
            assert task.error_code == "unsupported_message_schema"
            assert reservation is not None and reservation.status == "released"
            assert release is not None and release.attempt_id is None
            assert attempt_count == 0
            assert inbox is not None and inbox.status == "manual_attention"
            assert inbox.attempt_count == 2
        await channel.close()
    finally:
        for message in received:
            if not message.processed:
                await message.nack(requeue=True)
        await publisher.close()
        await observer.close()


@pytest.mark.parametrize(
    ("queue_name", "routing_key", "capacity"),
    (
        (IO_QUEUE, "io.script.extract", IO_WORKER_MAX_IN_FLIGHT),
        (MEDIA_QUEUE, "media.probe", MEDIA_WORKER_MAX_IN_FLIGHT),
    ),
)
@pytest.mark.skipif(
    os.getenv("LANVERSE_RUN_RABBITMQ_CONTRACT") != "1",
    reason="set LANVERSE_RUN_RABBITMQ_CONTRACT=1 with an isolated RabbitMQ vhost",
)
@pytest.mark.asyncio
async def test_real_worker_prefetch_holds_capacity_plus_one_message_ready(
    queue_name: str,
    routing_key: str,
    capacity: int,
) -> None:
    rabbitmq_url = rabbitmq_contract_url()
    publisher = RabbitMQPublisher(rabbitmq_url)
    observer = await aio_pika.connect_robust(rabbitmq_url, timeout=3)
    trace_id = f"prefetch-contract-{queue_name}"
    release = asyncio.Event()
    capacity_started = asyncio.Event()
    all_acked = asyncio.Event()
    received: list[aio_pika.abc.AbstractIncomingMessage] = []
    consumer_tag: str | None = None
    channel: aio_pika.abc.AbstractChannel | None = None
    queue: aio_pika.abc.AbstractQueue | None = None

    async def hold_message(message: aio_pika.abc.AbstractIncomingMessage) -> None:
        received.append(message)
        if len(received) == capacity:
            capacity_started.set()
        await release.wait()
        await message.ack()
        if len(received) == capacity + 1 and all(item.processed for item in received):
            all_acked.set()

    try:
        await publisher.connect()
        channel = await observer.channel()
        await channel.set_qos(prefetch_count=capacity)
        queue = await channel.declare_queue(queue_name, durable=True)
        if queue.declaration_result.message_count != 0:
            pytest.skip(f"{queue_name} is not empty; refusing to consume existing messages")
        consumer_tag = await queue.consume(hold_message, no_ack=False)

        for _ in range(capacity + 1):
            await publisher.publish(_capacity_envelope(trace_id), routing_key)

        await asyncio.wait_for(capacity_started.wait(), timeout=3)
        await asyncio.sleep(0.05)
        assert len(received) == capacity
        inspection_channel = await observer.channel()
        queue_state = await inspection_channel.declare_queue(
            queue_name,
            durable=True,
            passive=True,
        )
        assert queue_state.declaration_result.message_count == 1
        await inspection_channel.close()

        release.set()
        await asyncio.wait_for(all_acked.wait(), timeout=3)
        assert len(received) == capacity + 1
        inspection_channel = await observer.channel()
        final_state = await inspection_channel.declare_queue(
            queue_name,
            durable=True,
            passive=True,
        )
        assert final_state.declaration_result.message_count == 0
        await inspection_channel.close()
    finally:
        release.set()
        if queue is not None and consumer_tag is not None:
            await queue.cancel(consumer_tag)
        for message in received:
            if not message.processed:
                await message.ack()
        if channel is not None and not channel.is_closed:
            await channel.close()
        await publisher.close()
        await observer.close()
