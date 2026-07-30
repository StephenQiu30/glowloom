import hashlib
from collections.abc import AsyncIterator
from datetime import UTC, datetime, timedelta
from typing import Any
from uuid import UUID

import httpx
import pytest
from app.modules.media.models import MediaObject, MediaVersion, UploadSession
from sqlalchemy import func, select
from sqlalchemy.ext.asyncio import AsyncSession, async_sessionmaker

from app.integrations.minio import MinioObjectStorage
from app.modules.media.storage import StorageObjectNotFound, StorageUnavailable
from app.modules.messaging.models import OutboxEvent
from app.modules.production.models import Task
from tests.support.identity_builders import register_identity_response


class StoredObjects:
    def __init__(self) -> None:
        self.objects: dict[str, tuple[bytes, str]] = {}
        self.upload_keys: list[str] = []
        self.download_keys: list[str] = []

    @property
    def latest_upload_key(self) -> str:
        return self.upload_keys[-1]

    def upload(self, data: bytes, mime_type: str) -> None:
        self.objects[self.latest_upload_key] = (data, mime_type)


@pytest.fixture
def stored_objects(monkeypatch: pytest.MonkeyPatch) -> StoredObjects:
    state = StoredObjects()

    async def presign_upload(
        _: MinioObjectStorage, object_key: str, expires_seconds: int
    ) -> str:
        assert 0 < expires_seconds <= 3600
        state.upload_keys.append(object_key)
        return f"https://private-storage.test/upload/{object_key}?signature=secret"

    async def presign_download(
        _: MinioObjectStorage, object_key: str, expires_seconds: int
    ) -> str:
        assert 0 < expires_seconds <= 900
        state.download_keys.append(object_key)
        return f"https://private-storage.test/download/{object_key}?signature=secret"

    async def stat_object(
        _: MinioObjectStorage, object_key: str
    ) -> tuple[int, str | None]:
        stored = state.objects.get(object_key)
        if stored is None:
            raise StorageObjectNotFound("object not found")
        return len(stored[0]), stored[1]

    def stream_object(
        _: MinioObjectStorage, object_key: str
    ) -> AsyncIterator[bytes]:
        async def chunks() -> AsyncIterator[bytes]:
            stored = state.objects.get(object_key)
            if stored is None:
                raise StorageObjectNotFound("object not found")
            midpoint = max(1, len(stored[0]) // 2)
            yield stored[0][:midpoint]
            yield stored[0][midpoint:]

        return chunks()

    monkeypatch.setattr(MinioObjectStorage, "presign_upload", presign_upload)
    monkeypatch.setattr(MinioObjectStorage, "presign_download", presign_download)
    monkeypatch.setattr(MinioObjectStorage, "stat", stat_object)
    monkeypatch.setattr(MinioObjectStorage, "stream", stream_object)
    return state


async def _identity(
    client: httpx.AsyncClient,
    *,
    email: str = "media-owner@example.com",
) -> tuple[dict[str, str], UUID]:
    response = await register_identity_response(client, email=email)
    assert response.status_code == 201
    data = response.json()["data"]
    return (
        {"authorization": f"Bearer {data['access_token']}"},
        UUID(data["workspace"]["id"]),
    )


def _upload_request(
    workspace_id: UUID,
    content: bytes,
    *,
    idempotency_key: str,
    filename: str = "cover.png",
    kind: str = "image",
    mime_type: str = "image/png",
) -> dict[str, Any]:
    return {
        "workspace_id": str(workspace_id),
        "kind": kind,
        "filename": filename,
        "size_bytes": len(content),
        "mime_type": mime_type,
        "sha256": hashlib.sha256(content).hexdigest(),
        "idempotency_key": idempotency_key,
    }


async def _initialize_and_upload(
    client: httpx.AsyncClient,
    headers: dict[str, str],
    workspace_id: UUID,
    stored_objects: StoredObjects,
    content: bytes,
    *,
    idempotency_key: str,
) -> dict[str, Any]:
    response = await client.post(
        "/api/v1/media/uploads",
        headers=headers,
        json=_upload_request(
            workspace_id,
            content,
            idempotency_key=idempotency_key,
        ),
    )
    assert response.status_code == 201
    result = response.json()["data"]
    assert result["upload"]["method"] == "PUT"
    assert result["upload"]["headers"] == {"content-type": "image/png"}
    assert "private-storage.test" in result["upload"]["url"]
    assert "object_key" not in str(result)
    assert "bucket" not in str(result)
    stored_objects.upload(content, "image/png")
    return result


@pytest.mark.asyncio
async def test_direct_upload_completion_is_atomic_idempotent_and_privately_accessible(
    client: httpx.AsyncClient,
    session_factory: async_sessionmaker[AsyncSession],
    stored_objects: StoredObjects,
) -> None:
    content = b"not-a-real-image-yet-but-valid-upload-bytes"
    headers, workspace_id = await _identity(client)
    initialized = await _initialize_and_upload(
        client,
        headers,
        workspace_id,
        stored_objects,
        content,
        idempotency_key="media-upload-001",
    )

    completed = await client.post(
        f"/api/v1/media/uploads/{initialized['upload_session']['id']}/complete",
        headers=headers,
        json={},
    )
    assert completed.status_code == 200
    result = completed.json()["data"]
    media_object = result["media_object"]
    version = result["version"]
    probe_task = result["probe_task"]
    assert media_object["current_version_id"] == version["id"]
    assert media_object["kind"] == "image"
    assert media_object["status"] == "active"
    assert version["version_no"] == 1
    assert version["filename"] == "cover.png"
    assert version["sha256"] == hashlib.sha256(content).hexdigest()
    assert version["size_bytes"] == len(content)
    assert version["mime_type"] == "image/png"
    assert version["probe_status"] == "pending"
    assert probe_task["task_type"] == "media_probe"
    assert probe_task["request_type"] == "media_version"
    assert probe_task["request_id"] == version["id"]
    assert "url" not in str(version)

    repeated = await client.post(
        f"/api/v1/media/uploads/{initialized['upload_session']['id']}/complete",
        headers=headers,
        json={},
    )
    assert repeated.status_code == 200
    assert repeated.json()["data"] == result

    async with session_factory() as session:
        assert await session.scalar(select(func.count()).select_from(MediaObject)) == 1
        assert await session.scalar(select(func.count()).select_from(MediaVersion)) == 1
        assert await session.scalar(select(func.count()).select_from(Task)) == 1
        assert await session.scalar(select(func.count()).select_from(OutboxEvent)) == 1
        event = await session.scalar(select(OutboxEvent))
        assert event is not None
        assert event.event_type == "media_probe.requested"
        assert event.routing_key == "media.probe"
        assert event.payload == {"task_id": probe_task["id"]}

    listed = await client.get(
        "/api/v1/media",
        headers=headers,
        params={"workspace_id": str(workspace_id), "kind": "image"},
    )
    assert listed.status_code == 200
    assert listed.json()["data"]["items"] == [version]
    detail = await client.get(f"/api/v1/media/{version['id']}", headers=headers)
    assert detail.status_code == 200
    assert detail.json()["data"] == version
    access = await client.post(
        f"/api/v1/media/{version['id']}/access",
        headers=headers,
        json={"purpose": "preview"},
    )
    assert access.status_code == 200
    assert access.json()["data"]["method"] == "GET"
    assert "signature=secret" in access.json()["data"]["url"]
    assert stored_objects.download_keys == [stored_objects.latest_upload_key]

    other_headers, _ = await _identity(client, email="media-other@example.com")
    assert (
        await client.get(f"/api/v1/media/{version['id']}", headers=other_headers)
    ).status_code == 404
    assert (
        await client.post(
            f"/api/v1/media/{version['id']}/access",
            headers=other_headers,
            json={"purpose": "download"},
        )
    ).status_code == 404


@pytest.mark.asyncio
async def test_upload_rejects_invalid_declarations_and_never_creates_false_versions(
    client: httpx.AsyncClient,
    session_factory: async_sessionmaker[AsyncSession],
    stored_objects: StoredObjects,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    headers, workspace_id = await _identity(client)
    content = b"declared-media-content"
    request = _upload_request(
        workspace_id,
        content,
        idempotency_key="invalid-declaration",
    )

    incompatible = await client.post(
        "/api/v1/media/uploads",
        headers=headers,
        json={**request, "mime_type": "video/mp4"},
    )
    assert incompatible.status_code == 422
    assert incompatible.json()["error"]["code"] == "validation_failed"
    oversized = await client.post(
        "/api/v1/media/uploads",
        headers=headers,
        json={**request, "size_bytes": 2 * 1024 * 1024 * 1024 + 1},
    )
    assert oversized.status_code == 422

    initialized = await client.post(
        "/api/v1/media/uploads", headers=headers, json=request
    )
    assert initialized.status_code == 201
    upload_session_id = initialized.json()["data"]["upload_session"]["id"]
    stored_objects.upload(b"different-content", "image/png")
    mismatch = await client.post(
        f"/api/v1/media/uploads/{upload_session_id}/complete",
        headers=headers,
        json={},
    )
    assert mismatch.status_code == 422
    assert mismatch.json()["error"]["code"] == "validation_failed"
    assert mismatch.json()["error"]["next_action"] == "upload_again"

    async with session_factory() as session:
        async with session.begin():
            failed = await session.get(UploadSession, UUID(upload_session_id))
            assert failed is not None
            assert failed.status == "failed"
            assert failed.error_code == "object_validation_failed"
            assert await session.scalar(select(func.count()).select_from(MediaVersion)) == 0
            failed.status = "pending"
            failed.error_code = None
            failed.expires_at = datetime.now(UTC) - timedelta(seconds=1)

    expired = await client.post(
        f"/api/v1/media/uploads/{upload_session_id}/complete",
        headers=headers,
        json={},
    )
    assert expired.status_code == 409
    assert expired.json()["error"]["next_action"] == "initialize_upload"

    second = await client.post(
        "/api/v1/media/uploads",
        headers=headers,
        json={**request, "idempotency_key": "storage-failure"},
    )
    assert second.status_code == 201

    async def unavailable(*_: object, **__: object) -> tuple[int, str | None]:
        raise StorageUnavailable("synthetic outage")

    monkeypatch.setattr(MinioObjectStorage, "stat", unavailable)
    unavailable_response = await client.post(
        f"/api/v1/media/uploads/{second.json()['data']['upload_session']['id']}/complete",
        headers=headers,
        json={},
    )
    assert unavailable_response.status_code == 503
    error = unavailable_response.json()["error"]
    assert error["code"] == "dependency_unavailable"
    assert error["details"] == {"retryable": True}
    async with session_factory() as session:
        pending = await session.get(
            UploadSession, UUID(second.json()["data"]["upload_session"]["id"])
        )
        assert pending is not None and pending.status == "pending"
        assert await session.scalar(select(func.count()).select_from(MediaVersion)) == 0


@pytest.mark.asyncio
async def test_append_version_uses_current_pointer_cas_and_archive_preserves_history(
    client: httpx.AsyncClient,
    session_factory: async_sessionmaker[AsyncSession],
    stored_objects: StoredObjects,
) -> None:
    headers, workspace_id = await _identity(client)
    first_content = b"media-version-one"
    initialized = await _initialize_and_upload(
        client,
        headers,
        workspace_id,
        stored_objects,
        first_content,
        idempotency_key="media-version-1",
    )
    first = (
        await client.post(
            f"/api/v1/media/uploads/{initialized['upload_session']['id']}/complete",
            headers=headers,
            json={},
        )
    ).json()["data"]
    object_id = first["media_object"]["id"]
    first_version_id = first["version"]["id"]

    async def initialize_replacement(key: str, content: bytes) -> dict[str, Any]:
        response = await client.post(
            f"/api/v1/media-objects/{object_id}/versions",
            headers=headers,
            json={
                **_upload_request(
                    workspace_id,
                    content,
                    idempotency_key=key,
                    filename=f"{key}.png",
                ),
                "expected_current_version_id": first_version_id,
            },
        )
        assert response.status_code == 201
        result = response.json()["data"]
        stored_objects.upload(content, "image/png")
        return result

    winner = await initialize_replacement("media-version-2", b"media-version-two")
    stale = await initialize_replacement("media-version-stale", b"stale-version-three")
    winner_result = await client.post(
        f"/api/v1/media/uploads/{winner['upload_session']['id']}/complete",
        headers=headers,
        json={},
    )
    assert winner_result.status_code == 200
    assert winner_result.json()["data"]["version"]["version_no"] == 2
    current_version_id = winner_result.json()["data"]["version"]["id"]

    conflict = await client.post(
        f"/api/v1/media/uploads/{stale['upload_session']['id']}/complete",
        headers=headers,
        json={},
    )
    assert conflict.status_code == 409
    assert conflict.json()["error"]["code"] == "version_conflict"
    assert conflict.json()["error"]["details"]["current_version_id"] == current_version_id
    async with session_factory() as session:
        assert await session.scalar(select(func.count()).select_from(MediaVersion)) == 2

    archived = await client.post(
        f"/api/v1/media-objects/{object_id}/archive",
        headers=headers,
        json={"expected_revision": 2},
    )
    assert archived.status_code == 200
    archived_object = archived.json()["data"]
    assert archived_object["status"] == "archived"
    assert archived_object["revision"] == 3

    hidden = await client.get(
        "/api/v1/media",
        headers=headers,
        params={"workspace_id": str(workspace_id)},
    )
    assert hidden.status_code == 200
    assert hidden.json()["data"]["total"] == 0
    historical = await client.get(
        "/api/v1/media",
        headers=headers,
        params={"workspace_id": str(workspace_id), "include_archived": True},
    )
    assert historical.status_code == 200
    assert [item["version_no"] for item in historical.json()["data"]["items"]] == [2, 1]
    access = await client.post(
        f"/api/v1/media/{first_version_id}/access",
        headers=headers,
        json={"purpose": "download"},
    )
    assert access.status_code == 200
    cannot_replace = await client.post(
        f"/api/v1/media-objects/{object_id}/versions",
        headers=headers,
        json={
            **_upload_request(
                workspace_id,
                b"another-version",
                idempotency_key="archived-version",
            ),
            "expected_current_version_id": current_version_id,
        },
    )
    assert cannot_replace.status_code == 409
    assert (
        await client.delete(f"/api/v1/media/{first_version_id}", headers=headers)
    ).status_code == 405


@pytest.mark.asyncio
async def test_failed_probe_can_be_retried_without_reuploading_bytes(
    client: httpx.AsyncClient,
    session_factory: async_sessionmaker[AsyncSession],
    stored_objects: StoredObjects,
) -> None:
    headers, workspace_id = await _identity(client)
    content = b"probe-retry-content"
    initialized = await _initialize_and_upload(
        client,
        headers,
        workspace_id,
        stored_objects,
        content,
        idempotency_key="probe-retry-upload",
    )
    completed = await client.post(
        f"/api/v1/media/uploads/{initialized['upload_session']['id']}/complete",
        headers=headers,
        json={},
    )
    assert completed.status_code == 200
    version_id = completed.json()["data"]["version"]["id"]
    initial_task_id = completed.json()["data"]["probe_task"]["id"]

    async with session_factory() as session:
        async with session.begin():
            version = await session.get(MediaVersion, UUID(version_id))
            task = await session.get(Task, UUID(initial_task_id))
            assert version is not None and task is not None
            version.probe_status = "failed"
            version.probe_error_code = "synthetic_failure"
            version.probe_error_summary = "Synthetic probe failure"
            version.probe_next_action = "retry_probe"
            task.status = "failed"
            task.progress_stage = "blocked"
            task.error_code = "synthetic_failure"
            task.error_retryable = True

    retried = await client.post(
        f"/api/v1/media/{version_id}/probe-retry",
        headers=headers,
        json={"idempotency_key": "probe-retry-command-001"},
    )
    assert retried.status_code == 202
    retry_task = retried.json()["data"]
    assert retry_task["id"] != initial_task_id
    assert retry_task["task_type"] == "media_probe"
    repeated = await client.post(
        f"/api/v1/media/{version_id}/probe-retry",
        headers=headers,
        json={"idempotency_key": "probe-retry-command-001"},
    )
    assert repeated.status_code == 202
    assert repeated.json()["data"] == retry_task
    assert stored_objects.upload_keys == [stored_objects.latest_upload_key]

    async with session_factory() as session:
        version = await session.get(MediaVersion, UUID(version_id))
        assert version is not None
        assert version.probe_status == "pending"
        assert version.probe_attempt == 2
        assert await session.scalar(select(func.count()).select_from(Task)) == 2
        assert await session.scalar(select(func.count()).select_from(OutboxEvent)) == 2
