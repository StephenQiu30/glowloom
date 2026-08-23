import hashlib
from collections.abc import AsyncIterator
from datetime import UTC, datetime, timedelta
from uuid import UUID

import pytest
from sqlalchemy import select
from sqlalchemy.ext.asyncio import AsyncSession, async_sessionmaker
from uuid6 import uuid7

from app import media_worker
from app.modules.identity import ActorContext
from app.modules.identity.models import UserAccount, Workspace
from app.modules.media import MediaProbeResult
from app.modules.media.models import MediaLocation, MediaObject, MediaVersion
from app.modules.media.storage import (
    ObjectStoragePort,
    StorageObjectMetadata,
    StorageObjectNotFound,
    StorageUnavailable,
)
from app.modules.messaging import envelope_from_event
from app.modules.messaging.models import OutboxEvent
from app.modules.production import (
    MediaLocationMigrationTaskCommand,
    MediaLocationRetirementTaskCommand,
    create_media_location_migration_task,
    create_media_location_retirement_task,
)
from app.modules.production.models import Task
from app.modules.scheduling.models import Schedule


class MemoryLocationStorage(ObjectStoragePort):
    def __init__(self, source_key: str, content: bytes) -> None:
        self.objects = {source_key: content}
        self.copied: list[tuple[str, str]] = []
        self.deleted: list[str] = []
        self.corrupt_copy = False
        self.copy_unavailable = False

    async def ensure_bucket(self) -> None:
        return None

    async def presign_upload(self, object_key: str, expires_seconds: int) -> str:
        raise AssertionError("migration worker must not presign uploads")

    async def presign_download(self, object_key: str, expires_seconds: int) -> str:
        raise AssertionError("migration worker must not presign downloads")

    async def stat(self, object_key: str) -> StorageObjectMetadata:
        content = self.objects.get(object_key)
        if content is None:
            raise StorageObjectNotFound("object not found")
        return StorageObjectMetadata(len(content), "video/mp4", "memory-etag")

    async def put(self, object_key: str, data: bytes, content_type: str) -> None:
        raise AssertionError("migration must use server-side copy")

    async def copy(self, source_key: str, target_key: str) -> None:
        if self.copy_unavailable:
            raise StorageUnavailable("storage unavailable")
        content = self.objects.get(source_key)
        if content is None:
            raise StorageObjectNotFound("object not found")
        self.objects[target_key] = b"corrupt" if self.corrupt_copy else content
        self.copied.append((source_key, target_key))

    def stream(self, object_key: str) -> AsyncIterator[bytes]:
        async def chunks() -> AsyncIterator[bytes]:
            content = self.objects.get(object_key)
            if content is None:
                raise StorageObjectNotFound("object not found")
            midpoint = max(1, len(content) // 2)
            yield content[:midpoint]
            yield content[midpoint:]

        return chunks()

    async def delete(self, object_key: str) -> None:
        if object_key not in self.objects:
            raise StorageObjectNotFound("object not found")
        del self.objects[object_key]
        self.deleted.append(object_key)


class UnusedProbe:
    async def probe(
        self,
        content: AsyncIterator[bytes],
        *,
        kind: str,
        mime_type: str,
    ) -> MediaProbeResult:
        raise AssertionError("location jobs must not invoke the media probe")


class RecordingMessage:
    def __init__(self, body: bytes) -> None:
        self.body = body
        self.ack_count = 0
        self.nack_requeues: list[bool] = []

    async def ack(self) -> None:
        self.ack_count += 1

    async def nack(self, *, requeue: bool) -> None:
        self.nack_requeues.append(requeue)


async def _location_fixture(
    factory: async_sessionmaker[AsyncSession],
    *,
    idempotency_key: str,
) -> tuple[ActorContext, UUID, UUID, str, bytes, bytes]:
    user_id = uuid7()
    workspace_id = uuid7()
    media_object_id = uuid7()
    version_id = uuid7()
    source_location_id = uuid7()
    source_key = f"workspaces/{workspace_id}/media/{version_id}/original"
    content = b"immutable-media-version-bytes"
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
                        email_normalized=f"location-{user_id}@example.com",
                        password_hash="synthetic-not-used",
                        display_name="Location Fixture",
                    ),
                    Workspace(id=workspace_id, name="Location Fixture"),
                    MediaObject(
                        id=media_object_id,
                        workspace_id=workspace_id,
                        kind="video",
                        source_type="upload",
                        status="active",
                        current_version_id=version_id,
                        revision=1,
                    ),
                    MediaVersion(
                        id=version_id,
                        workspace_id=workspace_id,
                        media_object_id=media_object_id,
                        version_no=1,
                        filename="episode.mp4",
                        sha256=hashlib.sha256(content).hexdigest(),
                        size_bytes=len(content),
                        mime_type="video/mp4",
                        probe_status="ready",
                        probe_attempt=1,
                        created_by=user_id,
                    ),
                    MediaLocation(
                        id=source_location_id,
                        workspace_id=workspace_id,
                        media_version_id=version_id,
                        storage_profile="default",
                        bucket="lanverse-media",
                        object_key=source_key,
                        status="active",
                        verified_at=datetime.now(UTC),
                    ),
                )
            )
            dispatch = await create_media_location_migration_task(
                session,
                MediaLocationMigrationTaskCommand(
                    workspace_id=workspace_id,
                    media_version_id=version_id,
                    location_id=source_location_id,
                    operation="migrate",
                    requested_by=user_id,
                    idempotency_key=idempotency_key,
                ),
                trace_id="location-migration-test",
            )
        event = await session.get(OutboxEvent, dispatch.outbox_event_id)
        assert event is not None
        body = envelope_from_event(event).model_dump_json().encode()
    return actor, version_id, source_location_id, source_key, content, body


@pytest.mark.asyncio
async def test_location_migration_verifies_bytes_and_atomically_switches_active(
    session_factory: async_sessionmaker[AsyncSession],
) -> None:
    _, version_id, source_id, source_key, content, body = await _location_fixture(
        session_factory,
        idempotency_key="location-migrate-success",
    )
    storage = MemoryLocationStorage(source_key, content)

    first_message = RecordingMessage(body)
    result = await media_worker.process_incoming_message(
        first_message,
        session_factory,
        storage=storage,
        probe=UnusedProbe(),
        storage_profile="default",
        storage_bucket="lanverse-media",
        location_rollback_seconds=3600,
    )
    assert result == "completed"
    assert first_message.ack_count == 1
    assert len(storage.copied) == 1

    duplicate_message = RecordingMessage(body)
    assert (
        await media_worker.process_incoming_message(
            duplicate_message,
            session_factory,
            storage=storage,
            probe=UnusedProbe(),
            storage_profile="default",
            storage_bucket="lanverse-media",
            location_rollback_seconds=3600,
        )
        == "duplicate"
    )
    assert duplicate_message.ack_count == 1
    assert len(storage.copied) == 1

    async with session_factory() as session:
        version = await session.get(MediaVersion, version_id)
        locations = list(
            await session.scalars(
                select(MediaLocation)
                .where(MediaLocation.media_version_id == version_id)
                .order_by(MediaLocation.created_at, MediaLocation.id)
            )
        )
        assert version is not None
        assert version.sha256 == hashlib.sha256(content).hexdigest()
        assert version.size_bytes == len(content)
        assert len(locations) == 2
        source = next(item for item in locations if item.id == source_id)
        target = next(item for item in locations if item.id != source_id)
        assert source.status == "retiring"
        assert source.retire_after is not None
        assert source.retire_after > datetime.now(UTC)
        assert target.status == "active"
        assert target.migration_task_id is not None
        assert target.retire_after is None
        schedule = await session.scalar(
            select(Schedule).where(Schedule.schedule_key == f"media.location.retire:{source.id}")
        )
        assert schedule is not None
        assert schedule.handler_name == "retire_media_location"
        assert schedule.next_fire_at == source.retire_after
        task = await session.scalar(select(Task).where(Task.id == target.migration_task_id))
        assert task is not None and task.status == "succeeded"
        assert storage.objects[target.object_key] == content


@pytest.mark.asyncio
async def test_location_migration_integrity_failure_keeps_original_active(
    session_factory: async_sessionmaker[AsyncSession],
) -> None:
    _, version_id, source_id, source_key, content, body = await _location_fixture(
        session_factory,
        idempotency_key="location-migrate-corrupt",
    )
    storage = MemoryLocationStorage(source_key, content)
    storage.corrupt_copy = True

    assert (
        await media_worker.process_incoming_message(
            RecordingMessage(body),
            session_factory,
            storage=storage,
            probe=UnusedProbe(),
            storage_profile="default",
            storage_bucket="lanverse-media",
            location_rollback_seconds=3600,
        )
        == "completed"
    )
    async with session_factory() as session:
        locations = list(
            await session.scalars(
                select(MediaLocation).where(MediaLocation.media_version_id == version_id)
            )
        )
        assert [(item.id, item.status) for item in locations] == [(source_id, "active")]
        task = await session.scalar(
            select(Task).where(Task.idempotency_key == "location-migrate-corrupt")
        )
        assert task is not None
        assert task.status == "failed"
        assert task.error_code == "media_location_integrity_failed"
        assert task.error_retryable is False
        assert source_key in storage.objects


@pytest.mark.asyncio
async def test_location_migration_storage_failure_requeues_without_database_switch(
    session_factory: async_sessionmaker[AsyncSession],
) -> None:
    _, version_id, source_id, source_key, content, body = await _location_fixture(
        session_factory,
        idempotency_key="location-migrate-storage-failure",
    )
    storage = MemoryLocationStorage(source_key, content)
    storage.copy_unavailable = True
    message = RecordingMessage(body)

    assert (
        await media_worker.process_incoming_message(
            message,
            session_factory,
            storage=storage,
            probe=UnusedProbe(),
            storage_profile="default",
            storage_bucket="lanverse-media",
            location_rollback_seconds=3600,
        )
        == "requeued"
    )
    assert message.ack_count == 0
    assert message.nack_requeues == [True]
    async with session_factory() as session:
        locations = list(
            await session.scalars(
                select(MediaLocation).where(MediaLocation.media_version_id == version_id)
            )
        )
        assert [(location.id, location.status) for location in locations] == [(source_id, "active")]
        task = await session.scalar(
            select(Task).where(Task.idempotency_key == "location-migrate-storage-failure")
        )
        assert task is not None and task.status == "queued"


@pytest.mark.asyncio
async def test_competing_location_task_cannot_replace_the_winner_active_location(
    session_factory: async_sessionmaker[AsyncSession],
) -> None:
    actor, version_id, source_id, source_key, content, winner_body = await _location_fixture(
        session_factory,
        idempotency_key="location-migrate-winner",
    )
    async with session_factory() as session:
        async with session.begin():
            loser = await create_media_location_migration_task(
                session,
                MediaLocationMigrationTaskCommand(
                    workspace_id=actor.workspace_id,
                    media_version_id=version_id,
                    location_id=source_id,
                    operation="migrate",
                    requested_by=actor.user_id,
                    idempotency_key="location-migrate-loser",
                ),
                trace_id="location-migration-loser",
            )
        event = await session.get(OutboxEvent, loser.outbox_event_id)
        assert event is not None
        loser_body = envelope_from_event(event).model_dump_json().encode()

    storage = MemoryLocationStorage(source_key, content)
    for body, expected in ((winner_body, "completed"), (loser_body, "rejected")):
        assert (
            await media_worker.process_incoming_message(
                RecordingMessage(body),
                session_factory,
                storage=storage,
                probe=UnusedProbe(),
                storage_profile="default",
                storage_bucket="lanverse-media",
                location_rollback_seconds=3600,
            )
            == expected
        )
    async with session_factory() as session:
        active_locations = list(
            await session.scalars(
                select(MediaLocation).where(
                    MediaLocation.media_version_id == version_id,
                    MediaLocation.status == "active",
                )
            )
        )
        assert len(active_locations) == 1
        loser_task = await session.scalar(
            select(Task).where(Task.idempotency_key == "location-migrate-loser")
        )
        assert loser_task is not None
        assert loser_task.status == "failed"
        assert loser_task.error_code == "media_location_changed"


@pytest.mark.asyncio
async def test_location_rollback_restores_retiring_bytes_and_restarts_window(
    session_factory: async_sessionmaker[AsyncSession],
) -> None:
    actor, version_id, source_id, source_key, content, body = await _location_fixture(
        session_factory,
        idempotency_key="location-migrate-before-rollback",
    )
    storage = MemoryLocationStorage(source_key, content)
    message = RecordingMessage(body)
    assert (
        await media_worker.process_incoming_message(
            message,
            session_factory,
            storage=storage,
            probe=UnusedProbe(),
            storage_profile="default",
            storage_bucket="lanverse-media",
            location_rollback_seconds=3600,
        )
        == "completed"
    )
    async with session_factory() as session:
        async with session.begin():
            active = await session.scalar(
                select(MediaLocation).where(
                    MediaLocation.media_version_id == version_id,
                    MediaLocation.status == "active",
                )
            )
            assert active is not None
            dispatch = await create_media_location_migration_task(
                session,
                MediaLocationMigrationTaskCommand(
                    workspace_id=actor.workspace_id,
                    media_version_id=version_id,
                    location_id=source_id,
                    operation="rollback",
                    requested_by=actor.user_id,
                    idempotency_key="location-rollback-success",
                ),
                trace_id="location-rollback-test",
            )
        event = await session.get(OutboxEvent, dispatch.outbox_event_id)
        assert event is not None
        rollback_body = envelope_from_event(event).model_dump_json().encode()
        previous_active_id = active.id

    assert (
        await media_worker.process_incoming_message(
            RecordingMessage(rollback_body),
            session_factory,
            storage=storage,
            probe=UnusedProbe(),
            storage_profile="default",
            storage_bucket="lanverse-media",
            location_rollback_seconds=3600,
        )
        == "completed"
    )
    async with session_factory() as session:
        restored = await session.get(MediaLocation, source_id)
        previous_active = await session.get(MediaLocation, previous_active_id)
        assert restored is not None and restored.status == "active"
        assert restored.retire_after is None
        assert previous_active is not None and previous_active.status == "retiring"
        assert previous_active.retire_after is not None
        assert previous_active.retire_after > datetime.now(UTC)


@pytest.mark.asyncio
async def test_location_retirement_rejects_early_delete_then_deletes_when_due(
    session_factory: async_sessionmaker[AsyncSession],
) -> None:
    actor, _, source_id, source_key, content, body = await _location_fixture(
        session_factory,
        idempotency_key="location-migrate-before-retirement",
    )
    storage = MemoryLocationStorage(source_key, content)
    assert (
        await media_worker.process_incoming_message(
            RecordingMessage(body),
            session_factory,
            storage=storage,
            probe=UnusedProbe(),
            storage_profile="default",
            storage_bucket="lanverse-media",
            location_rollback_seconds=3600,
        )
        == "completed"
    )

    async with session_factory() as session:
        async with session.begin():
            early = await create_media_location_retirement_task(
                session,
                MediaLocationRetirementTaskCommand(
                    workspace_id=actor.workspace_id,
                    media_location_id=source_id,
                    requested_by=actor.user_id,
                    idempotency_key="location-retire-early",
                ),
                trace_id="location-retire-early",
            )
        event = await session.get(OutboxEvent, early.outbox_event_id)
        assert event is not None
        early_body = envelope_from_event(event).model_dump_json().encode()
    assert (
        await media_worker.process_incoming_message(
            RecordingMessage(early_body),
            session_factory,
            storage=storage,
            probe=UnusedProbe(),
            storage_profile="default",
            storage_bucket="lanverse-media",
            location_rollback_seconds=3600,
        )
        == "completed"
    )
    assert source_key in storage.objects

    async with session_factory() as session:
        async with session.begin():
            source = await session.get(MediaLocation, source_id, with_for_update=True)
            assert source is not None
            source.retire_after = datetime.now(UTC) - timedelta(seconds=1)
            due = await create_media_location_retirement_task(
                session,
                MediaLocationRetirementTaskCommand(
                    workspace_id=actor.workspace_id,
                    media_location_id=source_id,
                    requested_by=actor.user_id,
                    idempotency_key="location-retire-due",
                ),
                trace_id="location-retire-due",
            )
        event = await session.get(OutboxEvent, due.outbox_event_id)
        assert event is not None
        due_body = envelope_from_event(event).model_dump_json().encode()
    assert (
        await media_worker.process_incoming_message(
            RecordingMessage(due_body),
            session_factory,
            storage=storage,
            probe=UnusedProbe(),
            storage_profile="default",
            storage_bucket="lanverse-media",
            location_rollback_seconds=3600,
        )
        == "completed"
    )
    async with session_factory() as session:
        source = await session.get(MediaLocation, source_id)
        assert source is not None and source.status == "retired"
        assert source.retired_at is not None
        assert source.retire_after is None
        assert source_key not in storage.objects
