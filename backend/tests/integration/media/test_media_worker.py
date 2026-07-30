import hashlib
from collections.abc import AsyncIterator, Awaitable, Callable
from uuid import UUID

import pytest
from app.modules.media.models import MediaLocation, MediaObject, MediaVersion
from sqlalchemy import select
from sqlalchemy.ext.asyncio import AsyncSession, async_sessionmaker
from uuid6 import uuid7

from app import media_worker
from app.modules.identity import ActorContext
from app.modules.identity.models import UserAccount, Workspace
from app.modules.media import MediaProbeError, MediaProbeResult
from app.modules.media.storage import ObjectStoragePort
from app.modules.messaging import envelope_from_event
from app.modules.messaging.models import InboxDelivery, OutboxEvent
from app.modules.production import MediaProbeTaskCommand, create_media_probe_task
from app.modules.production.models import Task


class MemoryStorage(ObjectStoragePort):
    def __init__(self, key: str, content: bytes) -> None:
        self.key = key
        self.content = content

    async def ensure_bucket(self) -> None:
        return None

    async def presign_upload(self, object_key: str, expires_seconds: int) -> str:
        raise AssertionError("worker must not presign uploads")

    async def presign_download(self, object_key: str, expires_seconds: int) -> str:
        raise AssertionError("worker must not presign downloads")

    async def stat(self, object_key: str) -> tuple[int, str | None]:
        return len(self.content), "video/mp4"

    async def put(self, object_key: str, data: bytes, content_type: str) -> None:
        raise AssertionError("worker must not overwrite media")

    def stream(self, object_key: str) -> AsyncIterator[bytes]:
        async def chunks() -> AsyncIterator[bytes]:
            assert object_key == self.key
            yield self.content

        return chunks()

    async def delete(self, object_key: str) -> None:
        raise AssertionError("worker must not delete confirmed media")


class RecordingProbe:
    def __init__(self, result: MediaProbeResult | Exception) -> None:
        self.result = result
        self.calls = 0

    async def probe(
        self,
        content: AsyncIterator[bytes],
        *,
        kind: str,
        mime_type: str,
    ) -> MediaProbeResult:
        self.calls += 1
        received = b"".join([chunk async for chunk in content])
        assert received == b"worker-media-bytes"
        assert kind == "video"
        assert mime_type == "video/mp4"
        if isinstance(self.result, Exception):
            raise self.result
        return self.result


class RecordingMessage:
    def __init__(
        self,
        body: bytes,
        *,
        on_ack: Callable[[], Awaitable[None]] | None = None,
    ) -> None:
        self.body = body
        self.on_ack = on_ack
        self.ack_count = 0
        self.nack_requeues: list[bool] = []

    async def ack(self) -> None:
        if self.on_ack is not None:
            await self.on_ack()
        self.ack_count += 1

    async def nack(self, *, requeue: bool) -> None:
        self.nack_requeues.append(requeue)


async def _pending_probe(
    factory: async_sessionmaker[AsyncSession],
) -> tuple[UUID, UUID, bytes, MemoryStorage]:
    user_id = uuid7()
    workspace_id = uuid7()
    media_object_id = uuid7()
    version_id = uuid7()
    location_id = uuid7()
    key = f"workspaces/{workspace_id}/media/{uuid7()}"
    now_hash = hashlib.sha256(b"worker-media-bytes").hexdigest()
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
                        email_normalized=f"media-worker-{user_id}@example.com",
                        password_hash="synthetic-not-used",
                        display_name="Media Worker Fixture",
                    ),
                    Workspace(id=workspace_id, name="Media Worker Fixture"),
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
                        filename="clip.mp4",
                        sha256=now_hash,
                        size_bytes=len(b"worker-media-bytes"),
                        mime_type="video/mp4",
                        probe_status="pending",
                        probe_attempt=1,
                        created_by=user_id,
                    ),
                    MediaLocation(
                        id=location_id,
                        workspace_id=workspace_id,
                        media_version_id=version_id,
                        storage_profile="default",
                        bucket="lanverse-media",
                        object_key=key,
                        status="active",
                    ),
                )
            )
            task = await create_media_probe_task(
                session,
                actor,
                MediaProbeTaskCommand(
                    workspace_id=workspace_id,
                    media_version_id=version_id,
                    idempotency_key=f"media-probe:{version_id}:1",
                ),
                trace_id="media-worker-test",
            )
        event = await session.scalar(
            select(OutboxEvent).where(OutboxEvent.aggregate_id == task.id)
        )
        assert event is not None
        body = envelope_from_event(event).model_dump_json().encode()
    return task.id, version_id, body, MemoryStorage(key, b"worker-media-bytes")


@pytest.mark.asyncio
async def test_media_worker_commits_probe_before_ack_and_is_idempotent(
    session_factory: async_sessionmaker[AsyncSession],
) -> None:
    task_id, version_id, body, storage = await _pending_probe(session_factory)
    probe = RecordingProbe(
        MediaProbeResult(
            width=1080,
            height=1920,
            duration_ms=3250,
            codec="h264",
            container="mov,mp4,m4a,3gp,3g2,mj2",
        )
    )

    async def assert_committed() -> None:
        async with session_factory() as session:
            version = await session.get(MediaVersion, version_id)
            task = await session.get(Task, task_id)
            delivery = await session.scalar(select(InboxDelivery))
            assert version is not None
            assert version.probe_status == "ready"
            assert version.width == 1080
            assert version.height == 1920
            assert version.duration_ms == 3250
            assert version.codec == "h264"
            assert task is not None and task.status == "succeeded"
            assert delivery is not None and delivery.status == "completed"

    first = RecordingMessage(body, on_ack=assert_committed)
    assert (
        await media_worker.process_incoming_message(
            first,
            session_factory,
            storage=storage,
            probe=probe,
        )
        == "completed"
    )
    assert first.ack_count == 1
    assert first.nack_requeues == []

    duplicate = RecordingMessage(body)
    assert (
        await media_worker.process_incoming_message(
            duplicate,
            session_factory,
            storage=storage,
            probe=probe,
        )
        == "duplicate"
    )
    assert duplicate.ack_count == 1
    assert probe.calls == 1


@pytest.mark.asyncio
async def test_probe_failure_keeps_confirmed_bytes_and_can_be_retried(
    session_factory: async_sessionmaker[AsyncSession],
) -> None:
    task_id, version_id, body, storage = await _pending_probe(session_factory)
    probe = RecordingProbe(MediaProbeError("unsupported_media", "Unable to inspect media"))
    message = RecordingMessage(body)
    assert (
        await media_worker.process_incoming_message(
            message,
            session_factory,
            storage=storage,
            probe=probe,
        )
        == "completed"
    )
    async with session_factory() as session:
        version = await session.get(MediaVersion, version_id)
        task = await session.get(Task, task_id)
        location = await session.scalar(
            select(MediaLocation).where(MediaLocation.media_version_id == version_id)
        )
        assert version is not None
        assert version.probe_status == "failed"
        assert version.probe_error_code == "unsupported_media"
        assert version.probe_next_action == "retry_probe"
        assert task is not None and task.status == "failed"
        assert task.error_retryable is True
        assert location is not None and location.status == "active"
