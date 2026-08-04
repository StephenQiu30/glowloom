import asyncio
import os
from datetime import UTC, datetime

import aio_pika
import pytest
from sqlalchemy import select
from sqlalchemy.ext.asyncio import async_sessionmaker
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
from app.modules.production.models import Task
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
