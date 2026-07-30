import os

import aio_pika
import pytest
from sqlalchemy import select
from sqlalchemy.ext.asyncio import async_sessionmaker
from uuid6 import uuid7

from app.core.database import Base, create_engine, validate_test_database_url
from app.integrations.rabbitmq import IO_QUEUE, RabbitMQPublisher
from app.io_worker import process_incoming_message
from app.modules.identity import ActorContext
from app.modules.identity.models import UserAccount, Workspace
from app.modules.messaging import envelope_from_event
from app.modules.messaging.models import InboxDelivery, OutboxEvent
from app.modules.production import ScriptExtractionTaskCommand, create_script_extraction_task
from app.modules.production.models import Task


@pytest.mark.skipif(
    os.getenv("LANVERSE_RUN_RABBITMQ_CONTRACT") != "1",
    reason="set LANVERSE_RUN_RABBITMQ_CONTRACT=1 with RabbitMQ running",
)
@pytest.mark.asyncio
async def test_worker_commits_inbox_before_manual_ack_on_real_queue() -> None:
    rabbitmq_url = os.getenv(
        "RABBITMQ_URL", "amqp://guest:guest@127.0.0.1:5672/"
    )
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
