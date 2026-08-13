import io
import json
import zipfile
from collections.abc import AsyncIterator
from uuid import UUID

import httpx
import pytest
from sqlalchemy import func, select
from sqlalchemy.ext.asyncio import AsyncSession, async_sessionmaker

from app import media_worker
from app.modules.assets.models import Asset
from app.modules.media import MediaProbePort
from app.modules.media.models import MediaLineage, MediaLocation, MediaObject, MediaVersion
from app.modules.media.storage import (
    ObjectStoragePort,
    StorageObjectMetadata,
    StorageUnavailable,
)
from app.modules.messaging import envelope_from_event
from app.modules.messaging.models import InboxDelivery, OutboxEvent
from app.modules.production.models import Task
from app.modules.storyboards.exports.models import (
    StoryboardExportJob,
    StoryboardExportManifest,
)
from tests.integration.media.test_media_worker import RecordingMessage
from tests.integration.storyboards.test_storyboard_exports import (
    create_ready_export_episode,
)


class ExportStorage(ObjectStoragePort):
    def __init__(self, *, stat_failures: int = 0) -> None:
        self.objects: dict[str, tuple[bytes, str]] = {}
        self.put_calls: list[str] = []
        self.stat_failures = stat_failures

    async def ensure_bucket(self) -> None:
        return None

    async def presign_upload(self, object_key: str, expires_seconds: int) -> str:
        raise AssertionError("export worker must not presign uploads")

    async def presign_download(self, object_key: str, expires_seconds: int) -> str:
        raise AssertionError("export worker must not presign downloads")

    async def stat(self, object_key: str) -> StorageObjectMetadata:
        if self.stat_failures:
            self.stat_failures -= 1
            raise StorageUnavailable("temporary object storage failure")
        content, media_type = self.objects[object_key]
        return StorageObjectMetadata(len(content), media_type, "memory-etag")

    async def put(self, object_key: str, data: bytes, content_type: str) -> None:
        existing = self.objects.get(object_key)
        if existing is not None and existing != (data, content_type):
            raise AssertionError("deterministic object key received different bytes")
        self.objects[object_key] = (data, content_type)
        self.put_calls.append(object_key)

    async def copy(self, source_key: str, target_key: str) -> None:
        raise AssertionError("export worker must not copy objects")

    def stream(self, object_key: str) -> AsyncIterator[bytes]:
        async def chunks() -> AsyncIterator[bytes]:
            yield self.objects[object_key][0]

        return chunks()

    async def delete(self, object_key: str) -> None:
        raise AssertionError("export worker must not delete trusted packages")


class NoProbe(MediaProbePort):
    async def probe(
        self,
        content: AsyncIterator[bytes],
        *,
        kind: str,
        mime_type: str,
    ) -> object:
        raise AssertionError("storyboard export must not invoke media probing")


async def _requested_export(
    client: httpx.AsyncClient,
    session_factory: async_sessionmaker[AsyncSession],
    *,
    email: str,
) -> tuple[dict[str, str], dict[str, object], dict[str, UUID], bytes]:
    headers, episode, refs, asset_version = await create_ready_export_episode(
        client,
        session_factory,
        email=email,
    )
    preflight = await client.post(
        f"/api/v1/episodes/{episode['id']}/storyboard-exports/preflight",
        headers=headers,
    )
    assert preflight.status_code == 200
    created = await client.post(
        f"/api/v1/episodes/{episode['id']}/storyboard-exports",
        headers=headers,
        json={
            "expected_input_hash": preflight.json()["data"]["input_hash"],
            "idempotency_key": f"export-worker-{email}",
        },
    )
    assert created.status_code == 202
    export = created.json()["data"]
    async with session_factory() as session:
        event = await session.scalar(
            select(OutboxEvent).where(
                OutboxEvent.aggregate_id == UUID(export["task_id"])
            )
        )
        assert event is not None
        body = envelope_from_event(event).model_dump_json().encode()
    return headers, {"episode": episode, "export": export, "asset": asset_version}, refs, body


@pytest.mark.asyncio
async def test_export_worker_publishes_fixed_package_and_lineage_once(
    client: httpx.AsyncClient,
    session_factory: async_sessionmaker[AsyncSession],
) -> None:
    headers, values, refs, body = await _requested_export(
        client,
        session_factory,
        email="storyboard-export-worker@example.com",
    )
    asset_data = values["asset"]
    assert isinstance(asset_data, dict)
    async with session_factory() as session, session.begin():
        asset = await session.get(Asset, UUID(asset_data["asset_id"]), with_for_update=True)
        assert asset is not None
        asset.availability = "disabled"
        asset.revision += 1

    storage = ExportStorage()
    message = RecordingMessage(body)
    assert (
        await media_worker.process_incoming_message(
            message,
            session_factory,
            storage=storage,
            probe=NoProbe(),
        )
        == "completed"
    )
    assert message.ack_count == 1
    assert len(storage.objects) == 1
    object_key, (content, media_type) = next(iter(storage.objects.items()))
    assert object_key.startswith(f"exports/{refs['workspace_id']}/")
    assert media_type == "application/zip"
    with zipfile.ZipFile(io.BytesIO(content)) as package:
        snapshot = json.loads(package.read("storyboard.json"))["snapshot"]
        assert snapshot["asset_version_ids"] if "asset_version_ids" in snapshot else snapshot["assets"]
        assert snapshot["assets"][0]["asset_version_id"] == asset_data["id"]

    export_data = values["export"]
    assert isinstance(export_data, dict)
    job_id = UUID(export_data["id"])
    task_id = UUID(export_data["task_id"])
    async with session_factory() as session:
        job = await session.get(StoryboardExportJob, job_id)
        task = await session.get(Task, task_id)
        manifest = await session.scalar(
            select(StoryboardExportManifest).where(
                StoryboardExportManifest.job_id == job_id
            )
        )
        assert job is not None and job.status == "succeeded"
        assert task is not None and task.status == "succeeded"
        assert manifest is not None
        version = await session.get(MediaVersion, manifest.media_version_id)
        assert version is not None and version.probe_status == "ready"
        media_object = await session.get(MediaObject, version.media_object_id)
        assert media_object is not None and media_object.kind == "delivery"
        location = await session.scalar(
            select(MediaLocation).where(MediaLocation.media_version_id == version.id)
        )
        assert location is not None and location.object_key == object_key
        lineage_types = set(
            await session.scalars(
                select(MediaLineage.source_type).where(
                    MediaLineage.media_version_id == version.id
                )
            )
        )
        assert {
            "asset_version",
            "narrative_unit_version",
            "script_version",
            "shot_spec_version",
            "storyboard_coverage",
            "storyboard_export_snapshot",
            "storyboard_readiness",
        } <= lineage_types

    duplicate = RecordingMessage(body)
    assert (
        await media_worker.process_incoming_message(
            duplicate,
            session_factory,
            storage=storage,
            probe=NoProbe(),
        )
        == "duplicate"
    )
    async with session_factory() as session:
        assert await session.scalar(select(func.count()).select_from(StoryboardExportManifest)) == 1
        assert await session.scalar(select(func.count()).select_from(MediaVersion)) == 2

    episode = values["episode"]
    assert isinstance(episode, dict)
    history = await client.get(
        f"/api/v1/episodes/{episode['id']}/storyboard-exports",
        headers=headers,
    )
    assert history.status_code == 200
    assert history.json()["data"]["items"][0]["status"] == "succeeded"
    assert history.json()["data"]["items"][0]["manifest"]["package_sha256"]


@pytest.mark.asyncio
async def test_export_worker_retries_same_bytes_after_storage_failure(
    client: httpx.AsyncClient,
    session_factory: async_sessionmaker[AsyncSession],
) -> None:
    _headers, values, _refs, body = await _requested_export(
        client,
        session_factory,
        email="storyboard-export-worker-retry@example.com",
    )
    storage = ExportStorage(stat_failures=1)

    first = RecordingMessage(body)
    assert (
        await media_worker.process_incoming_message(
            first,
            session_factory,
            storage=storage,
            probe=NoProbe(),
        )
        == "requeued"
    )
    assert first.nack_requeues == [True]
    first_key, (first_bytes, _media_type) = next(iter(storage.objects.items()))
    async with session_factory() as session:
        assert await session.scalar(select(func.count()).select_from(StoryboardExportManifest)) == 0
        assert await session.scalar(select(func.count()).select_from(InboxDelivery)) == 0

    second = RecordingMessage(body)
    assert (
        await media_worker.process_incoming_message(
            second,
            session_factory,
            storage=storage,
            probe=NoProbe(),
        )
        == "completed"
    )
    assert storage.put_calls == [first_key, first_key]
    assert storage.objects[first_key][0] == first_bytes
    export_data = values["export"]
    assert isinstance(export_data, dict)
    async with session_factory() as session:
        job = await session.get(StoryboardExportJob, UUID(export_data["id"]))
        assert job is not None and job.status == "succeeded"
