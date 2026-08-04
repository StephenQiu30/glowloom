import hashlib
import os
from datetime import UTC, datetime, timedelta
from typing import Any, cast
from uuid import UUID

import aio_pika
import httpx
import pytest
from sqlalchemy import func, select
from sqlalchemy.ext.asyncio import AsyncSession, async_sessionmaker

from app.integrations.minio import MinioObjectStorage
from app.integrations.rabbitmq import MEDIA_QUEUE, RabbitMQPublisher
from app.media_worker import process_incoming_message
from app.modules.media import MediaProbePort
from app.modules.media.storage import ObjectStoragePort
from app.modules.messaging import envelope_from_event
from app.modules.messaging.models import InboxDelivery, OutboxEvent
from app.modules.production.models import Task
from app.modules.scheduling import repository
from app.modules.scheduling.dispatcher import dispatch_due_schedules
from app.modules.scheduling.models import Schedule, ScheduleFire
from app.scheduler import publish_outbox_batch
from tests.support.external_contracts import rabbitmq_contract_url
from tests.support.identity_builders import register_identity_response


class RabbitUnavailable(RuntimeError):
    pass


class UnavailablePublisher:
    async def publish(self, *_: Any, **__: Any) -> None:
        raise RabbitUnavailable("connection details must not be persisted")


class RecordingStorage:
    def __init__(self) -> None:
        self.deleted: list[str] = []

    async def delete(self, object_key: str) -> None:
        self.deleted.append(object_key)


class UnusedProbe:
    async def probe(self, *_: Any, **__: Any) -> None:
        raise AssertionError("upload cleanup must not invoke ffprobe")


@pytest.mark.skipif(
    os.getenv("LANVERSE_RUN_SCHEDULER_STACK_CONTRACT") != "1",
    reason="set LANVERSE_RUN_SCHEDULER_STACK_CONTRACT=1 with an isolated RabbitMQ vhost",
)
@pytest.mark.asyncio
async def test_cron_lease_outbox_and_worker_recover_once_on_real_stack(
    client: httpx.AsyncClient,
    session_factory: async_sessionmaker[AsyncSession],
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    async def presign_upload(
        _: MinioObjectStorage, object_key: str, expires_seconds: int
    ) -> str:
        return f"https://storage.invalid/{object_key}?expires={expires_seconds}"

    monkeypatch.setattr(MinioObjectStorage, "presign_upload", presign_upload)
    rabbitmq_url = rabbitmq_contract_url()
    publisher = RabbitMQPublisher(rabbitmq_url)
    observer = await aio_pika.connect_robust(rabbitmq_url, timeout=3)
    messages: list[aio_pika.abc.AbstractIncomingMessage] = []
    try:
        channel = await observer.channel()
        queue = await channel.declare_queue(MEDIA_QUEUE, durable=True)
        assert queue.declaration_result.message_count == 0

        identity = await register_identity_response(
            client, email="scheduler-recovery-stack@example.com"
        )
        assert identity.status_code == 201
        identity_data = identity.json()["data"]
        headers = {"authorization": f"Bearer {identity_data['access_token']}"}
        workspace_id = UUID(identity_data["workspace"]["id"])
        content = b"scheduler-recovery-stack"
        initialized = await client.post(
            "/api/v1/media/uploads",
            headers=headers,
            json={
                "workspace_id": str(workspace_id),
                "kind": "image",
                "filename": "scheduler-recovery.png",
                "size_bytes": len(content),
                "mime_type": "image/png",
                "sha256": hashlib.sha256(content).hexdigest(),
                "idempotency_key": "scheduler-recovery-stack",
            },
        )
        assert initialized.status_code == 201

        async with session_factory() as session:
            schedule = await session.scalar(
                select(Schedule).where(
                    Schedule.workspace_id == workspace_id,
                    Schedule.handler_name == "cleanup_expired_uploads",
                )
            )
            assert schedule is not None
            schedule_id = schedule.id
            schedule_revision = schedule.revision

        real_now = datetime.now(UTC)
        configured = await client.put(
            f"/api/v1/schedules/{schedule_id}/configuration",
            headers=headers,
            json={
                "expected_revision": schedule_revision,
                "effective_from": (real_now - timedelta(minutes=5)).isoformat(),
                "kind": "cron",
                "cron_expression": "* * * * *",
                "timezone": "UTC",
                "misfire_policy": "run_once",
                "max_catch_up": 0,
                "misfire_grace_seconds": 30,
            },
        )
        assert configured.status_code == 200

        crashed_at = real_now - timedelta(seconds=60)
        due_at = crashed_at.replace(second=0, microsecond=0) - timedelta(minutes=1)
        async with session_factory() as session:
            async with session.begin():
                stored = await session.get(Schedule, schedule_id, with_for_update=True)
                assert stored is not None
                stored.next_fire_at = due_at
        async with session_factory() as session:
            async with session.begin():
                assert await repository.claim_due_schedules(
                    session,
                    dispatcher_id="crashed-scheduler-process",
                    now=crashed_at,
                    batch_size=10,
                    lease_duration=timedelta(seconds=30),
                ) == [schedule_id]

        assert await dispatch_due_schedules(
            session_factory,
            dispatcher_id="early-restarted-scheduler",
            now=crashed_at + timedelta(seconds=29),
            batch_size=10,
            lease_duration=timedelta(seconds=30),
        ) == 0
        recovered_at = crashed_at + timedelta(seconds=31)
        assert await dispatch_due_schedules(
            session_factory,
            dispatcher_id="restarted-scheduler",
            now=recovered_at,
            batch_size=10,
            lease_duration=timedelta(seconds=30),
        ) == 1
        assert await dispatch_due_schedules(
            session_factory,
            dispatcher_id="duplicate-restarted-scheduler",
            now=recovered_at,
            batch_size=10,
            lease_duration=timedelta(seconds=30),
        ) == 0

        assert await publish_outbox_batch(
            session_factory,
            UnavailablePublisher(),
            publisher_id="unavailable-rabbit-publisher",
            batch_size=10,
            claim_timeout=timedelta(seconds=30),
        ) == 0
        async with session_factory() as session:
            event = await session.scalar(
                select(OutboxEvent).where(
                    OutboxEvent.workspace_id == workspace_id,
                    OutboxEvent.event_type == "upload_cleanup.requested",
                )
            )
            assert event is not None
            assert event.status == "pending"
            assert event.attempt_count == 1
            assert event.last_error == "RabbitUnavailable"
            assert "connection details" not in event.last_error
            event_id = event.id
        async with session_factory() as session:
            async with session.begin():
                event = await session.get(OutboxEvent, event_id, with_for_update=True)
                assert event is not None
                event.available_at = datetime.now(UTC) - timedelta(seconds=1)

        await publisher.connect()
        assert await publish_outbox_batch(
            session_factory,
            publisher,
            publisher_id="recovered-rabbit-publisher",
            batch_size=10,
            claim_timeout=timedelta(seconds=30),
        ) == 1
        message = await queue.get(timeout=3, fail=False)
        assert message is not None
        messages.append(message)
        storage = RecordingStorage()
        assert await process_incoming_message(
            message,
            session_factory,
            storage=cast(ObjectStoragePort, storage),
            probe=cast(MediaProbePort, UnusedProbe()),
        ) == "completed"
        assert storage.deleted == []

        async with session_factory() as session:
            event = await session.get(OutboxEvent, event_id)
            assert event is not None and event.status == "published"
            envelope = envelope_from_event(event)
        await publisher.publish(envelope, "media.upload.cleanup")
        duplicate = await queue.get(timeout=3, fail=False)
        assert duplicate is not None
        messages.append(duplicate)
        assert await process_incoming_message(
            duplicate,
            session_factory,
            storage=cast(ObjectStoragePort, storage),
            probe=cast(MediaProbePort, UnusedProbe()),
        ) == "duplicate"

        async with session_factory() as session:
            schedule = await session.get(Schedule, schedule_id)
            task = await session.scalar(
                select(Task).where(
                    Task.workspace_id == workspace_id,
                    Task.task_type == "upload_cleanup",
                )
            )
            inbox = await session.scalar(
                select(InboxDelivery).where(InboxDelivery.event_id == event_id)
            )
            assert schedule is not None and schedule.next_fire_at is not None
            assert schedule.next_fire_at > recovered_at
            assert task is not None and task.status == "succeeded"
            assert inbox is not None and inbox.status == "completed"
            assert inbox.attempt_count == 2
            assert await session.scalar(
                select(func.count())
                .select_from(ScheduleFire)
                .where(ScheduleFire.schedule_id == schedule_id)
            ) == 1
            assert await session.scalar(
                select(func.count())
                .select_from(Task)
                .where(
                    Task.workspace_id == workspace_id,
                    Task.task_type == "upload_cleanup",
                )
            ) == 1
        await channel.close()
    finally:
        for message in messages:
            if not message.processed:
                await message.nack(requeue=True)
        await publisher.close()
        await observer.close()
