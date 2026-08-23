import hashlib
import io
import os
import zipfile
from collections.abc import AsyncIterator
from datetime import UTC, datetime, timedelta
from typing import cast
from urllib.parse import unquote, urlparse
from uuid import UUID

import httpx
import pytest
from sqlalchemy import func, select
from sqlalchemy.ext.asyncio import AsyncSession, async_sessionmaker

from app.core.config import Settings
from app.integrations.minio import MinioObjectStorage
from app.modules.media import MediaProbePort, MediaProbeResult
from app.modules.media.expiration_consumer import consume_upload_expiration
from app.modules.media.models import MediaVersion, UploadSession
from app.modules.media.storage import StorageObjectNotFound
from app.modules.messaging import envelope_from_event
from app.modules.messaging.models import OutboxEvent
from app.modules.production.models import Task
from app.modules.scheduling.dispatcher import dispatch_due_schedules
from app.modules.scheduling.models import Schedule
from app.modules.storyboards.exports.models import StoryboardExportManifest
from app.runtime.workers import media as media_worker
from tests.integration.storyboards.test_storyboard_exports import (
    create_ready_export_episode,
)
from tests.support.identity_builders import register_identity_response


class ContractMessage:
    def __init__(self, body: bytes) -> None:
        self.body = body
        self.ack_count = 0
        self.nack_requeues: list[bool] = []

    async def ack(self) -> None:
        self.ack_count += 1

    async def nack(self, *, requeue: bool) -> None:
        self.nack_requeues.append(requeue)


class NoDeliveryProbe(MediaProbePort):
    async def probe(
        self,
        content: AsyncIterator[bytes],
        *,
        kind: str,
        mime_type: str,
    ) -> MediaProbeResult:
        raise AssertionError("storyboard delivery packages do not require probing")


@pytest.mark.skipif(
    os.getenv("LANVERSE_RUN_MINIO_CONTRACT") != "1",
    reason="set LANVERSE_RUN_MINIO_CONTRACT=1 with the explicit MinIO profile running",
)
@pytest.mark.asyncio
async def test_media_api_completes_a_real_private_minio_upload(
    client: httpx.AsyncClient,
    test_settings: Settings,
) -> None:
    storage = MinioObjectStorage(
        test_settings.minio_endpoint,
        test_settings.minio_access_key,
        test_settings.minio_secret_key,
        test_settings.minio_bucket,
        secure=test_settings.minio_secure,
        thread_limit=test_settings.storage_thread_limit,
    )
    await storage.ensure_bucket()
    readiness = await client.get("/readyz")
    assert readiness.json()["dependencies"]["minio"]["status"] == "available"
    identity = await register_identity_response(client, email="media-minio-contract@example.com")
    assert identity.status_code == 201
    identity_data = identity.json()["data"]
    headers = {"authorization": f"Bearer {identity_data['access_token']}"}
    workspace_id = identity_data["workspace"]["id"]
    content = b"real-minio-private-media-contract"
    initialized = await client.post(
        "/api/v1/media/uploads",
        headers=headers,
        json={
            "workspace_id": workspace_id,
            "kind": "image",
            "filename": "contract.png",
            "size_bytes": len(content),
            "mime_type": "image/png",
            "sha256": hashlib.sha256(content).hexdigest(),
            "idempotency_key": "real-minio-media-contract",
        },
    )
    assert initialized.status_code == 201
    result = initialized.json()["data"]
    upload_url = cast(str, result["upload"]["url"])
    path = unquote(urlparse(upload_url).path)
    prefix = f"/{test_settings.minio_bucket}/"
    assert path.startswith(prefix)
    object_key = path.removeprefix(prefix)
    try:
        async with httpx.AsyncClient() as external:
            uploaded = await external.put(
                upload_url,
                content=content,
                headers=result["upload"]["headers"],
            )
        assert uploaded.status_code == 200

        completed = await client.post(
            f"/api/v1/media/uploads/{result['upload_session']['id']}/complete",
            headers=headers,
            json={},
        )
        assert completed.status_code == 200
        version = completed.json()["data"]["version"]
        assert version["sha256"] == hashlib.sha256(content).hexdigest()
        assert version["size_bytes"] == len(content)

        access = await client.post(
            f"/api/v1/media/{version['id']}/access",
            headers=headers,
            json={"purpose": "download"},
        )
        assert access.status_code == 200
        async with httpx.AsyncClient() as external:
            downloaded = await external.get(access.json()["data"]["url"])
        assert downloaded.status_code == 200
        assert downloaded.content == content
    finally:
        await storage.delete(object_key)


@pytest.mark.skipif(
    os.getenv("LANVERSE_RUN_MINIO_CONTRACT") != "1",
    reason="set LANVERSE_RUN_MINIO_CONTRACT=1 with the explicit MinIO profile running",
)
@pytest.mark.asyncio
async def test_storyboard_export_uses_private_minio_and_controlled_download(
    client: httpx.AsyncClient,
    session_factory: async_sessionmaker[AsyncSession],
    test_settings: Settings,
) -> None:
    storage = MinioObjectStorage(
        test_settings.minio_endpoint,
        test_settings.minio_access_key,
        test_settings.minio_secret_key,
        test_settings.minio_bucket,
        secure=test_settings.minio_secure,
        thread_limit=test_settings.storage_thread_limit,
    )
    await storage.ensure_bucket()
    headers, episode, refs, _asset_version = await create_ready_export_episode(
        client,
        session_factory,
        email="storyboard-export-minio@example.com",
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
            "idempotency_key": "storyboard-export-real-minio",
        },
    )
    assert created.status_code == 202
    export = created.json()["data"]
    async with session_factory() as session:
        event = await session.scalar(
            select(OutboxEvent).where(OutboxEvent.aggregate_id == UUID(export["task_id"]))
        )
        assert event is not None
        body = envelope_from_event(event).model_dump_json().encode()

    object_key = f"exports/{refs['workspace_id']}/{export['id']}.zip"
    message = ContractMessage(body)
    try:
        assert (
            await media_worker.process_incoming_message(
                message,
                session_factory,
                storage=storage,
                probe=NoDeliveryProbe(),
            )
            == "completed"
        )
        assert message.ack_count == 1
        package_bytes = b"".join([chunk async for chunk in storage.stream(object_key)])
        with zipfile.ZipFile(io.BytesIO(package_bytes)) as package:
            assert package.namelist() == [
                "manifest.json",
                "storyboard.csv",
                "storyboard.html",
                "storyboard.json",
            ]
        async with session_factory() as session:
            manifest = await session.scalar(
                select(StoryboardExportManifest).where(
                    StoryboardExportManifest.job_id == UUID(export["id"])
                )
            )
            assert manifest is not None
            media_version_id = manifest.media_version_id
        access = await client.post(
            f"/api/v1/media/{media_version_id}/access",
            headers=headers,
            json={"purpose": "download"},
        )
        assert access.status_code == 200
        scheme = "https" if test_settings.minio_secure else "http"
        anonymous_url = (
            f"{scheme}://{test_settings.minio_endpoint}/{test_settings.minio_bucket}/{object_key}"
        )
        async with httpx.AsyncClient(timeout=5) as external:
            downloaded = await external.get(access.json()["data"]["url"])
            anonymous = await external.get(anonymous_url)
        assert downloaded.status_code == 200
        assert downloaded.content == package_bytes
        assert anonymous.status_code in {401, 403}
    finally:
        await storage.delete(object_key)


@pytest.mark.skipif(
    os.getenv("LANVERSE_RUN_MINIO_CONTRACT") != "1",
    reason="set LANVERSE_RUN_MINIO_CONTRACT=1 with the explicit MinIO profile running",
)
@pytest.mark.asyncio
async def test_real_minio_hash_mismatch_never_creates_a_version_and_is_cleaned(
    client: httpx.AsyncClient,
    session_factory: async_sessionmaker[AsyncSession],
    test_settings: Settings,
) -> None:
    storage = MinioObjectStorage(
        test_settings.minio_endpoint,
        test_settings.minio_access_key,
        test_settings.minio_secret_key,
        test_settings.minio_bucket,
        secure=test_settings.minio_secure,
        thread_limit=test_settings.storage_thread_limit,
        operation_timeout_seconds=test_settings.storage_operation_timeout_seconds,
    )
    await storage.ensure_bucket()
    identity = await register_identity_response(
        client, email="media-minio-mismatch-contract@example.com"
    )
    assert identity.status_code == 201
    identity_data = identity.json()["data"]
    headers = {"authorization": f"Bearer {identity_data['access_token']}"}
    workspace_id = identity_data["workspace"]["id"]
    declared = b"declared-minio-contract"
    uploaded_bytes = b"x" * len(declared)
    assert len(uploaded_bytes) == len(declared)
    initialized = await client.post(
        "/api/v1/media/uploads",
        headers=headers,
        json={
            "workspace_id": workspace_id,
            "kind": "image",
            "filename": "mismatch.png",
            "size_bytes": len(declared),
            "mime_type": "image/png",
            "sha256": hashlib.sha256(declared).hexdigest(),
            "idempotency_key": "real-minio-mismatch-contract",
        },
    )
    assert initialized.status_code == 201
    result = initialized.json()["data"]
    upload_id = UUID(result["upload_session"]["id"])
    upload_url = cast(str, result["upload"]["url"])
    path = unquote(urlparse(upload_url).path)
    prefix = f"/{test_settings.minio_bucket}/"
    assert path.startswith(prefix)
    object_key = path.removeprefix(prefix)
    try:
        async with httpx.AsyncClient(timeout=5) as external:
            uploaded = await external.put(
                upload_url,
                content=uploaded_bytes,
                headers=result["upload"]["headers"],
            )
        assert uploaded.status_code == 200

        completed = await client.post(
            f"/api/v1/media/uploads/{upload_id}/complete",
            headers=headers,
            json={},
        )
        assert completed.status_code == 422
        assert completed.json()["error"]["next_action"] == "upload_again"
        async with session_factory() as session:
            async with session.begin():
                upload = await session.get(UploadSession, upload_id, with_for_update=True)
                assert upload is not None and upload.status == "failed"
                assert upload.error_code == "object_validation_failed"
                assert await session.scalar(select(func.count()).select_from(MediaVersion)) == 0
                schedule = await session.scalar(
                    select(Schedule).where(
                        Schedule.handler_name == "expire_upload_session",
                        Schedule.workspace_id == UUID(workspace_id),
                    )
                )
                assert schedule is not None
                schedule.next_fire_at = datetime.now(UTC) - timedelta(seconds=1)

        assert (
            await dispatch_due_schedules(
                session_factory,
                dispatcher_id="minio-mismatch-contract",
                now=datetime.now(UTC),
                batch_size=10,
                lease_duration=timedelta(seconds=30),
            )
            == 1
        )
        async with session_factory() as session:
            task = await session.scalar(
                select(Task).where(
                    Task.task_type == "upload_expiration",
                    Task.request_id == upload_id,
                )
            )
            assert task is not None
            event = await session.scalar(
                select(OutboxEvent).where(OutboxEvent.aggregate_id == task.id)
            )
            assert event is not None
            envelope = envelope_from_event(event)
        async with session_factory() as session:
            async with session.begin():
                assert (
                    await consume_upload_expiration(
                        session,
                        envelope,
                        storage=storage,
                    )
                    == "completed"
                )
        with pytest.raises(StorageObjectNotFound):
            await storage.stat(object_key)
    finally:
        await storage.delete(object_key)
