import asyncio
import hashlib
from datetime import UTC, datetime, timedelta
from typing import Any, cast
from uuid import UUID

import httpx
import pytest
from sqlalchemy import func, select
from sqlalchemy.ext.asyncio import AsyncSession, async_sessionmaker
from uuid6 import uuid7

from app.integrations.minio import MinioObjectStorage
from app.media_worker import process_incoming_message
from app.modules.governance.audit.models import AuditEvent
from app.modules.media import MediaProbePort
from app.modules.media.models import UploadSession
from app.modules.media.storage import ObjectStoragePort
from app.modules.messaging import envelope_from_event
from app.modules.messaging.models import InboxDelivery, OutboxEvent
from app.modules.production.models import Task
from app.modules.scheduling.dispatcher import dispatch_due_schedules
from app.modules.scheduling.models import Schedule, ScheduleFire
from tests.support.identity_builders import register_identity_response


class IncomingMessage:
    def __init__(self, body: bytes) -> None:
        self.body = body
        self.acked = 0
        self.requeued = 0

    async def ack(self) -> None:
        self.acked += 1

    async def nack(self, *, requeue: bool) -> None:
        self.requeued += int(requeue)


class RecordingStorage:
    def __init__(self) -> None:
        self.deleted: list[str] = []

    async def delete(self, object_key: str) -> None:
        self.deleted.append(object_key)


class UnusedProbe:
    async def probe(self, *_: Any, **__: Any) -> None:
        raise AssertionError("upload expiration must not invoke ffprobe")


async def _identity(
    client: httpx.AsyncClient,
    *,
    email: str,
) -> tuple[dict[str, str], UUID]:
    response = await register_identity_response(client, email=email)
    assert response.status_code == 201
    data = response.json()["data"]
    return (
        {"authorization": f"Bearer {data['access_token']}"},
        UUID(data["workspace"]["id"]),
    )


async def _initialize_upload(
    client: httpx.AsyncClient,
    headers: dict[str, str],
    workspace_id: UUID,
    *,
    idempotency_key: str,
) -> dict[str, Any]:
    content = b"scheduled-upload-expiration"
    response = await client.post(
        "/api/v1/media/uploads",
        headers=headers,
        json={
            "workspace_id": str(workspace_id),
            "kind": "image",
            "filename": "temporary.png",
            "size_bytes": len(content),
            "mime_type": "image/png",
            "sha256": hashlib.sha256(content).hexdigest(),
            "idempotency_key": idempotency_key,
        },
    )
    assert response.status_code == 201
    return response.json()["data"]


@pytest.mark.asyncio
async def test_upload_expiration_schedule_is_owned_operable_and_dispatched_once(
    client: httpx.AsyncClient,
    session_factory: async_sessionmaker[AsyncSession],
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    async def presign_upload(_: MinioObjectStorage, object_key: str, expires_seconds: int) -> str:
        assert expires_seconds > 0
        return f"https://storage.invalid/{object_key}"

    monkeypatch.setattr(MinioObjectStorage, "presign_upload", presign_upload)
    headers, workspace_id = await _identity(client, email="schedule-owner@example.com")
    other_headers, _ = await _identity(client, email="schedule-other@example.com")
    initialized = await _initialize_upload(
        client,
        headers,
        workspace_id,
        idempotency_key="scheduled-expiration-001",
    )
    upload_id = UUID(initialized["upload_session"]["id"])

    async with session_factory() as session:
        schedule = await session.scalar(
            select(Schedule).where(Schedule.handler_name == "expire_upload_session")
        )
        assert schedule is not None
        assert schedule.workspace_id == workspace_id
        assert schedule.schedule_key == f"media.upload.expire:{upload_id}"
        assert schedule.handler_name == "expire_upload_session"
        assert schedule.kind == "one_off"
        assert schedule.status == "active"
        assert schedule.next_fire_at == datetime.fromisoformat(
            initialized["upload_session"]["expires_at"].replace("Z", "+00:00")
        )
        assert schedule.scope == {
            "usage_type": "upload_session",
            "usage_id": str(upload_id),
        }
        assert schedule.payload == {"upload_session_id": str(upload_id)}
        one_off_due_at = schedule.next_fire_at
        assert one_off_due_at is not None
        assert await session.scalar(select(func.count()).select_from(Task)) == 0
        assert await session.scalar(select(func.count()).select_from(OutboxEvent)) == 0

    listed = await client.get(
        "/api/v1/schedules",
        headers=headers,
        params={"workspace_id": str(workspace_id)},
    )
    assert listed.status_code == 200
    page = listed.json()["data"]
    assert page["total"] == 2
    public_schedule = next(
        item for item in page["items"] if item["handler_name"] == "expire_upload_session"
    )
    assert public_schedule["handler_name"] == "expire_upload_session"
    assert "payload" not in public_schedule

    hidden = await client.get(
        "/api/v1/schedules",
        headers=other_headers,
        params={"workspace_id": str(workspace_id)},
    )
    assert hidden.status_code == 404

    paused = await client.post(
        f"/api/v1/schedules/{schedule.id}/pause",
        headers=headers,
        json={"expected_revision": schedule.revision},
    )
    assert paused.status_code == 200
    paused_schedule = paused.json()["data"]
    assert paused_schedule["status"] == "paused"
    assert (
        await dispatch_due_schedules(
            session_factory,
            dispatcher_id="dispatcher-paused",
            now=one_off_due_at + timedelta(seconds=1),
            batch_size=10,
            lease_duration=timedelta(seconds=30),
        )
        == 0
    )

    due_at = datetime.now(UTC) - timedelta(seconds=1)
    resumed = await client.post(
        f"/api/v1/schedules/{schedule.id}/resume",
        headers=headers,
        json={
            "expected_revision": paused_schedule["revision"],
            "resume_from": due_at.isoformat(),
            "misfire_policy": "run_once",
        },
    )
    assert resumed.status_code == 200

    results = await asyncio.gather(
        dispatch_due_schedules(
            session_factory,
            dispatcher_id="dispatcher-a",
            now=datetime.now(UTC),
            batch_size=10,
            lease_duration=timedelta(seconds=30),
        ),
        dispatch_due_schedules(
            session_factory,
            dispatcher_id="dispatcher-b",
            now=datetime.now(UTC),
            batch_size=10,
            lease_duration=timedelta(seconds=30),
        ),
    )
    assert sum(results) == 1

    async with session_factory() as session:
        schedule = await session.get(Schedule, schedule.id)
        fire = await session.scalar(select(ScheduleFire))
        task = await session.scalar(select(Task))
        event = await session.scalar(select(OutboxEvent))
        assert schedule is not None and schedule.status == "completed"
        assert schedule.next_fire_at is None
        assert fire is not None and fire.trigger_kind == "scheduled"
        assert task is not None and task.task_type == "upload_expiration"
        assert task.request_type == "upload_session"
        assert task.request_id == upload_id
        assert event is not None and event.event_type == "upload_expiration.requested"
        assert event.routing_key == "media.upload.expire"
        assert fire.task_id == task.id
        assert fire.outbox_event_id == event.id


@pytest.mark.asyncio
async def test_upload_expiration_worker_deletes_temporary_object_and_is_idempotent(
    client: httpx.AsyncClient,
    session_factory: async_sessionmaker[AsyncSession],
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    async def presign_upload(_: MinioObjectStorage, object_key: str, expires_seconds: int) -> str:
        return f"https://storage.invalid/{object_key}?expires={expires_seconds}"

    monkeypatch.setattr(MinioObjectStorage, "presign_upload", presign_upload)
    headers, workspace_id = await _identity(client, email="schedule-worker@example.com")
    initialized = await _initialize_upload(
        client,
        headers,
        workspace_id,
        idempotency_key="scheduled-expiration-worker",
    )
    upload_id = UUID(initialized["upload_session"]["id"])

    due_at = datetime.now(UTC) - timedelta(seconds=1)
    async with session_factory() as session:
        async with session.begin():
            upload = await session.get(UploadSession, upload_id, with_for_update=True)
            schedule = await session.scalar(
                select(Schedule).where(
                    Schedule.workspace_id == workspace_id,
                    Schedule.handler_name == "expire_upload_session",
                )
            )
            assert upload is not None and schedule is not None
            upload.expires_at = due_at
            schedule.next_fire_at = due_at

    assert (
        await dispatch_due_schedules(
            session_factory,
            dispatcher_id="dispatcher-worker",
            now=datetime.now(UTC),
            batch_size=10,
            lease_duration=timedelta(seconds=30),
        )
        == 1
    )
    async with session_factory() as session:
        event = await session.scalar(select(OutboxEvent))
        upload = await session.get(UploadSession, upload_id)
        assert event is not None and upload is not None
        object_key = upload.object_key
        message_body = envelope_from_event(event).model_dump_json().encode()

    storage = RecordingStorage()
    first = IncomingMessage(message_body)
    assert (
        await process_incoming_message(
            first,
            session_factory,
            storage=cast(ObjectStoragePort, storage),
            probe=cast(MediaProbePort, UnusedProbe()),
        )
        == "completed"
    )
    assert first.acked == 1
    assert first.requeued == 0
    assert storage.deleted == [object_key]

    duplicate = IncomingMessage(message_body)
    assert (
        await process_incoming_message(
            duplicate,
            session_factory,
            storage=cast(ObjectStoragePort, storage),
            probe=cast(MediaProbePort, UnusedProbe()),
        )
        == "duplicate"
    )
    assert duplicate.acked == 1
    assert storage.deleted == [object_key]

    async with session_factory() as session:
        upload = await session.get(UploadSession, upload_id)
        task = await session.scalar(select(Task))
        assert upload is not None and upload.status == "expired"
        assert upload.error_code == "upload_expired"
        assert task is not None and task.status == "succeeded"
        assert task.progress_stage == "completed"
        assert await session.scalar(select(func.count()).select_from(InboxDelivery)) == 1


@pytest.mark.asyncio
async def test_manual_schedule_trigger_is_authorized_and_idempotent(
    client: httpx.AsyncClient,
    session_factory: async_sessionmaker[AsyncSession],
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    async def presign_upload(_: MinioObjectStorage, object_key: str, expires_seconds: int) -> str:
        return f"https://storage.invalid/{object_key}?expires={expires_seconds}"

    monkeypatch.setattr(MinioObjectStorage, "presign_upload", presign_upload)
    headers, workspace_id = await _identity(client, email="schedule-trigger@example.com")
    other_headers, _ = await _identity(client, email="schedule-trigger-other@example.com")
    await _initialize_upload(
        client,
        headers,
        workspace_id,
        idempotency_key="manual-expiration-trigger",
    )
    async with session_factory() as session:
        schedule = await session.scalar(
            select(Schedule).where(Schedule.handler_name == "expire_upload_session")
        )
        assert schedule is not None
        schedule_id = schedule.id
        schedule_revision = schedule.revision

    payload = {
        "expected_revision": schedule_revision,
        "idempotency_key": "manual-cleanup-001",
    }
    first = await client.post(
        f"/api/v1/schedules/{schedule_id}/trigger",
        headers=headers,
        json=payload,
    )
    repeated = await client.post(
        f"/api/v1/schedules/{schedule_id}/trigger",
        headers=headers,
        json=payload,
    )
    assert first.status_code == 200
    assert repeated.status_code == 200
    assert repeated.json()["data"] == first.json()["data"]
    assert first.json()["data"]["trigger_kind"] == "manual"
    assert first.json()["data"]["task"]["task_type"] == "upload_expiration"

    hidden = await client.post(
        f"/api/v1/schedules/{schedule_id}/trigger",
        headers=other_headers,
        json=payload,
    )
    assert hidden.status_code == 404

    async with session_factory() as session:
        assert await session.scalar(select(func.count()).select_from(ScheduleFire)) == 1
        assert await session.scalar(select(func.count()).select_from(Task)) == 1
        assert await session.scalar(select(func.count()).select_from(OutboxEvent)) == 1
        assert (
            await session.scalar(
                select(func.count())
                .select_from(AuditEvent)
                .where(
                    AuditEvent.target_id == schedule_id,
                    AuditEvent.action == "schedule.triggered",
                )
            )
            == 1
        )


@pytest.mark.asyncio
async def test_workspace_cleanup_interval_is_unique_and_advances_past_missed_periods(
    client: httpx.AsyncClient,
    session_factory: async_sessionmaker[AsyncSession],
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    async def presign_upload(_: MinioObjectStorage, object_key: str, expires_seconds: int) -> str:
        return f"https://storage.invalid/{object_key}?expires={expires_seconds}"

    monkeypatch.setattr(MinioObjectStorage, "presign_upload", presign_upload)
    headers, workspace_id = await _identity(client, email="schedule-interval@example.com")
    initialized = await asyncio.gather(
        _initialize_upload(
            client,
            headers,
            workspace_id,
            idempotency_key="cleanup-interval-first",
        ),
        _initialize_upload(
            client,
            headers,
            workspace_id,
            idempotency_key="cleanup-interval-second",
        ),
    )
    assert len(initialized) == 2

    now = datetime.now(UTC)
    async with session_factory() as session:
        async with session.begin():
            cleanup_schedules = list(
                await session.scalars(
                    select(Schedule).where(
                        Schedule.workspace_id == workspace_id,
                        Schedule.handler_name == "cleanup_expired_uploads",
                    )
                )
            )
            assert len(cleanup_schedules) == 1
            schedule = cleanup_schedules[0]
            assert schedule.kind == "interval"
            assert schedule.scope == {
                "usage_type": "workspace",
                "usage_id": str(workspace_id),
            }
            assert schedule.payload == {"workspace_id": str(workspace_id)}
            interval_seconds = int(schedule.rule["seconds"])
            schedule.next_fire_at = now - timedelta(seconds=interval_seconds * 3)

    results = await asyncio.gather(
        dispatch_due_schedules(
            session_factory,
            dispatcher_id="interval-dispatcher-a",
            now=now,
            batch_size=10,
            lease_duration=timedelta(seconds=30),
        ),
        dispatch_due_schedules(
            session_factory,
            dispatcher_id="interval-dispatcher-b",
            now=now,
            batch_size=10,
            lease_duration=timedelta(seconds=30),
        ),
    )
    assert sum(results) == 1

    async with session_factory() as session:
        schedule = await session.get(Schedule, schedule.id)
        assert schedule is not None
        assert schedule.status == "active"
        assert schedule.next_fire_at is not None
        assert now < schedule.next_fire_at <= now + timedelta(seconds=interval_seconds)
        fire = await session.scalar(
            select(ScheduleFire).where(ScheduleFire.schedule_id == schedule.id)
        )
        task = await session.scalar(select(Task).where(Task.task_type == "upload_cleanup"))
        assert fire is not None and fire.trigger_kind == "scheduled"
        assert task is not None
        event = await session.scalar(select(OutboxEvent).where(OutboxEvent.aggregate_id == task.id))
        assert task.request_type == "workspace"
        assert task.request_id == workspace_id
        assert event is not None
        assert event.event_type == "upload_cleanup.requested"
        assert event.routing_key == "media.upload.cleanup"


@pytest.mark.asyncio
async def test_workspace_cleanup_worker_only_deletes_expired_uncompleted_uploads(
    client: httpx.AsyncClient,
    session_factory: async_sessionmaker[AsyncSession],
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    async def presign_upload(_: MinioObjectStorage, object_key: str, expires_seconds: int) -> str:
        return f"https://storage.invalid/{object_key}?expires={expires_seconds}"

    monkeypatch.setattr(MinioObjectStorage, "presign_upload", presign_upload)
    headers, workspace_id = await _identity(client, email="schedule-cleanup-worker@example.com")
    initialized = [
        await _initialize_upload(
            client,
            headers,
            workspace_id,
            idempotency_key=f"cleanup-worker-{index}",
        )
        for index in range(3)
    ]
    upload_ids = [UUID(item["upload_session"]["id"]) for item in initialized]
    now = datetime.now(UTC)
    async with session_factory() as session:
        async with session.begin():
            expired = await session.get(UploadSession, upload_ids[0], with_for_update=True)
            future = await session.get(UploadSession, upload_ids[1], with_for_update=True)
            completed = await session.get(UploadSession, upload_ids[2], with_for_update=True)
            cleanup_schedule = await session.scalar(
                select(Schedule).where(
                    Schedule.workspace_id == workspace_id,
                    Schedule.handler_name == "cleanup_expired_uploads",
                )
            )
            assert expired is not None and future is not None and completed is not None
            assert cleanup_schedule is not None
            expired.expires_at = now - timedelta(seconds=1)
            future.expires_at = now + timedelta(days=1)
            completed.expires_at = now - timedelta(seconds=1)
            completed.status = "completed"
            completed.completed_version_id = uuid7()
            cleanup_schedule.next_fire_at = now - timedelta(seconds=1)
            expected_deleted_key = expired.object_key
            protected_keys = {future.object_key, completed.object_key}

    assert (
        await dispatch_due_schedules(
            session_factory,
            dispatcher_id="cleanup-worker-dispatcher",
            now=now,
            batch_size=10,
            lease_duration=timedelta(seconds=30),
        )
        == 1
    )
    async with session_factory() as session:
        event = await session.scalar(
            select(OutboxEvent).where(OutboxEvent.event_type == "upload_cleanup.requested")
        )
        assert event is not None
        message_body = envelope_from_event(event).model_dump_json().encode()

    storage = RecordingStorage()
    message = IncomingMessage(message_body)
    assert (
        await process_incoming_message(
            message,
            session_factory,
            storage=cast(ObjectStoragePort, storage),
            probe=cast(MediaProbePort, UnusedProbe()),
        )
        == "completed"
    )
    assert message.acked == 1
    assert message.requeued == 0
    assert storage.deleted == [expected_deleted_key]
    assert protected_keys.isdisjoint(storage.deleted)

    duplicate = IncomingMessage(message_body)
    assert (
        await process_incoming_message(
            duplicate,
            session_factory,
            storage=cast(ObjectStoragePort, storage),
            probe=cast(MediaProbePort, UnusedProbe()),
        )
        == "duplicate"
    )
    assert duplicate.acked == 1
    assert storage.deleted == [expected_deleted_key]

    async with session_factory() as session:
        expired = await session.get(UploadSession, upload_ids[0])
        future = await session.get(UploadSession, upload_ids[1])
        completed = await session.get(UploadSession, upload_ids[2])
        task = await session.scalar(select(Task).where(Task.task_type == "upload_cleanup"))
        assert task is not None and task.status == "succeeded"
        audit = await session.scalar(
            select(AuditEvent).where(
                AuditEvent.target_id == task.id,
                AuditEvent.action == "task.succeeded",
            )
        )
        assert expired is not None and expired.status == "expired"
        assert future is not None and future.status == "pending"
        assert completed is not None and completed.status == "completed"
        assert completed.completed_version_id is not None
        assert audit is not None and audit.event_metadata["cleaned_count"] == 1


@pytest.mark.asyncio
async def test_workspace_cleanup_storage_failure_requeues_without_false_completion(
    client: httpx.AsyncClient,
    session_factory: async_sessionmaker[AsyncSession],
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    async def presign_upload(_: MinioObjectStorage, object_key: str, expires_seconds: int) -> str:
        return f"https://storage.invalid/{object_key}?expires={expires_seconds}"

    monkeypatch.setattr(MinioObjectStorage, "presign_upload", presign_upload)
    headers, workspace_id = await _identity(
        client, email="schedule-cleanup-storage-failure@example.com"
    )
    initialized = await _initialize_upload(
        client,
        headers,
        workspace_id,
        idempotency_key="cleanup-storage-failure",
    )
    upload_id = UUID(initialized["upload_session"]["id"])
    now = datetime.now(UTC)
    async with session_factory() as session:
        async with session.begin():
            upload = await session.get(UploadSession, upload_id, with_for_update=True)
            schedule = await session.scalar(
                select(Schedule).where(
                    Schedule.workspace_id == workspace_id,
                    Schedule.handler_name == "cleanup_expired_uploads",
                )
            )
            assert upload is not None and schedule is not None
            upload.expires_at = now - timedelta(seconds=1)
            schedule.next_fire_at = now - timedelta(seconds=1)

    assert (
        await dispatch_due_schedules(
            session_factory,
            dispatcher_id="cleanup-storage-failure-dispatcher",
            now=now,
            batch_size=10,
            lease_duration=timedelta(seconds=30),
        )
        == 1
    )
    async with session_factory() as session:
        event = await session.scalar(
            select(OutboxEvent).where(OutboxEvent.event_type == "upload_cleanup.requested")
        )
        assert event is not None
        message_body = envelope_from_event(event).model_dump_json().encode()

    class UnavailableStorage(RecordingStorage):
        async def delete(self, object_key: str) -> None:
            self.deleted.append(object_key)
            raise RuntimeError("storage unavailable")

    storage = UnavailableStorage()
    message = IncomingMessage(message_body)
    assert (
        await process_incoming_message(
            message,
            session_factory,
            storage=cast(ObjectStoragePort, storage),
            probe=cast(MediaProbePort, UnusedProbe()),
        )
        == "requeued"
    )
    assert message.acked == 0
    assert message.requeued == 1
    assert len(storage.deleted) == 1

    async with session_factory() as session:
        upload = await session.get(UploadSession, upload_id)
        task = await session.scalar(select(Task).where(Task.task_type == "upload_cleanup"))
        assert upload is not None and upload.status == "pending"
        assert task is not None and task.status == "queued"
        assert await session.scalar(select(func.count()).select_from(InboxDelivery)) == 0
