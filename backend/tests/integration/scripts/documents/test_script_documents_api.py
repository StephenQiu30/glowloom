import asyncio
import hashlib
from collections.abc import AsyncIterator
from typing import Any
from uuid import UUID

import httpx
import pytest
from fastapi import FastAPI
from sqlalchemy import func, select
from sqlalchemy.ext.asyncio import AsyncSession, async_sessionmaker

from app.integrations.dependencies import get_media_storage
from app.modules.media.models import MediaVersion
from app.modules.media.storage import MediaStorage, StorageObjectMetadata
from app.modules.messaging.models import OutboxEvent
from app.modules.projects.models import Episode
from app.modules.scripts.models import (
    DocumentRevision,
    FormatIssue,
    NarrativeBlock,
    ScriptDocument,
)
from tests.support.project_builders import project_payload, register_project_owner


class MemoryDocumentStorage:
    def __init__(self) -> None:
        self.objects: dict[str, tuple[bytes, str]] = {}
        self.upload_keys: list[str] = []

    async def ensure_bucket(self) -> None:
        return None

    async def presign_upload(self, object_key: str, expires_seconds: int) -> str:
        assert expires_seconds > 0
        self.upload_keys.append(object_key)
        return f"https://private-storage.test/upload/{len(self.upload_keys)}"

    async def presign_download(self, object_key: str, expires_seconds: int) -> str:
        assert object_key in self.objects
        assert expires_seconds > 0
        return "https://private-storage.test/download/signed"

    async def stat(self, object_key: str) -> StorageObjectMetadata:
        body, mime_type = self.objects[object_key]
        return StorageObjectMetadata(
            size_bytes=len(body),
            content_type=mime_type,
            etag="memory-etag",
        )

    async def put(self, object_key: str, data: bytes, content_type: str) -> None:
        self.objects[object_key] = (data, content_type)

    async def copy(self, source_key: str, target_key: str) -> None:
        self.objects[target_key] = self.objects[source_key]

    def stream(self, object_key: str) -> AsyncIterator[bytes]:
        async def chunks() -> AsyncIterator[bytes]:
            body, _ = self.objects[object_key]
            midpoint = max(1, len(body) // 2)
            yield body[:midpoint]
            yield body[midpoint:]

        return chunks()

    async def delete(self, object_key: str) -> None:
        self.objects.pop(object_key, None)


async def _project(
    client: httpx.AsyncClient,
    *,
    email: str,
) -> tuple[dict[str, str], str, dict[str, Any]]:
    headers, workspace_id = await register_project_owner(client, email=email)
    response = await client.post(
        "/api/v1/projects",
        headers=headers,
        json=project_payload(workspace_id, name="整剧导入验收项目"),
    )
    assert response.status_code == 201
    return headers, workspace_id, response.json()["data"]


def _text_payload(
    text: str = "第一集\n内景·控制室·夜\n甲：开始。\n\n第二集\n外景·港口·日\n乙：继续。",
    *,
    idempotency_key: str = "whole-script-import-001",
) -> dict[str, object]:
    return {
        "input_type": "text",
        "title": "整剧原稿",
        "text": text,
        "language": "zh-CN",
        "rights_declaration": "确认拥有该测试文本的使用权",
        "idempotency_key": idempotency_key,
    }


@pytest.mark.asyncio
async def test_text_import_is_exact_idempotent_refreshable_and_writes_no_episode(
    client: httpx.AsyncClient,
    session_factory: async_sessionmaker[AsyncSession],
) -> None:
    headers, _, project = await _project(
        client, email="whole-script-owner@example.com"
    )
    endpoint = f"/api/v1/projects/{project['id']}/script-imports"
    payload = _text_payload()

    created = await client.post(endpoint, headers=headers, json=payload)

    assert created.status_code == 201
    result = created.json()["data"]
    document = result["document"]
    revision = result["revision"]
    assert document["project_id"] == project["id"]
    assert document["source_type"] == "text"
    assert document["source_media_version_id"] is None
    assert "idempotency_key" not in document
    assert revision["document_id"] == document["id"]
    assert revision["version_no"] == 1
    assert revision["raw_text"] == payload["text"]
    assert revision["normalized_text"] == payload["text"]
    assert revision["raw_hash"] == hashlib.sha256(
        str(payload["text"]).encode()
    ).hexdigest()
    assert revision["normalized_hash"] == revision["raw_hash"]
    assert revision["codepoint_count"] == len(str(payload["text"]))
    assert revision["analysis_status"] == "deterministic"
    assert revision["normalizer_version"] == "identity-v1"
    assert result["issues"] == []
    assert "".join(
        str(payload["text"])[block["source_start"] : block["source_end"]]
        for block in result["blocks"]
    ) == payload["text"]

    repeated = await client.post(endpoint, headers=headers, json=payload)
    assert repeated.status_code == 201
    assert repeated.json()["data"] == result

    conflicting = await client.post(
        endpoint,
        headers=headers,
        json=_text_payload("第一集\n另一份正文。"),
    )
    assert conflicting.status_code == 409
    assert conflicting.json()["error"]["code"] == "resource_conflict"

    refreshed = await client.get(
        f"/api/v1/document-revisions/{revision['id']}", headers=headers
    )
    assert refreshed.status_code == 200
    assert refreshed.json()["data"] == result
    listed = await client.get(
        f"/api/v1/projects/{project['id']}/script-documents", headers=headers
    )
    assert listed.status_code == 200
    assert listed.json()["data"] == {
        "items": [document],
        "total": 1,
        "limit": 20,
        "offset": 0,
    }

    audit = await client.get(
        "/api/v1/audit-events",
        headers=headers,
        params={
            "workspace_id": document["workspace_id"],
            "target_type": "document_revision",
            "target_id": revision["id"],
        },
    )
    assert audit.status_code == 200
    assert audit.json()["data"]["total"] == 1
    serialized_audit = str(audit.json()["data"])
    assert "甲：开始" not in serialized_audit
    assert revision["raw_hash"] not in serialized_audit

    async with session_factory() as session:
        assert await session.scalar(select(func.count()).select_from(ScriptDocument)) == 1
        assert await session.scalar(select(func.count()).select_from(DocumentRevision)) == 1
        assert await session.scalar(select(func.count()).select_from(NarrativeBlock)) == len(
            result["blocks"]
        )
        assert await session.scalar(select(func.count()).select_from(FormatIssue)) == 0
        assert await session.scalar(select(func.count()).select_from(Episode)) == 0
        assert await session.scalar(select(func.count()).select_from(OutboxEvent)) == 0


@pytest.mark.asyncio
async def test_format_problems_are_persisted_with_next_actions_before_materialization(
    client: httpx.AsyncClient,
    session_factory: async_sessionmaker[AsyncSession],
) -> None:
    headers, _, project = await _project(
        client, email="whole-script-format@example.com"
    )
    endpoint = f"/api/v1/projects/{project['id']}/script-imports"

    rejected = await client.post(
        endpoint,
        headers=headers,
        json=_text_payload(
            "第一集\n正文。\n\n第三集\n正文。",
            idempotency_key="whole-script-gap",
        ),
    )
    candidate = await client.post(
        endpoint,
        headers=headers,
        json=_text_payload(
            "场景：控制室。\n甲：正文里提到第一集结束。",
            idempotency_key="whole-script-no-marker",
        ),
    )
    bom = await client.post(
        endpoint,
        headers=headers,
        json=_text_payload(
            "\ufeff第一集\n正文。",
            idempotency_key="whole-script-bom",
        ),
    )

    assert rejected.status_code == 201
    assert rejected.json()["data"]["revision"]["analysis_status"] == "rejected"
    assert [item["code"] for item in rejected.json()["data"]["issues"]] == [
        "number_gap"
    ]
    assert rejected.json()["data"]["issues"][0]["next_action"] == (
        "renumber_episode_markers"
    )
    assert candidate.status_code == 201
    assert candidate.json()["data"]["revision"]["analysis_status"] == (
        "ai_candidate_required"
    )
    assert candidate.json()["data"]["issues"][0]["code"] == "no_marker"
    assert bom.status_code == 201
    assert bom.json()["data"]["revision"]["analysis_status"] == "rejected"
    assert bom.json()["data"]["issues"][0]["code"] == "utf8_bom_not_allowed"

    blank = await client.post(
        endpoint,
        headers=headers,
        json=_text_payload(" \r\n ", idempotency_key="whole-script-blank"),
    )
    too_long = await client.post(
        endpoint,
        headers=headers,
        json=_text_payload("字" * 100_001, idempotency_key="whole-script-too-long"),
    )
    both_inputs = await client.post(
        endpoint,
        headers=headers,
        json=_text_payload(idempotency_key="whole-script-both")
        | {"media_version_id": "00000000-0000-0000-0000-000000000001"},
    )
    assert blank.status_code == 422
    assert too_long.status_code == 422
    assert both_inputs.status_code == 422

    async with session_factory() as session:
        assert await session.scalar(select(func.count()).select_from(ScriptDocument)) == 3
        assert await session.scalar(select(func.count()).select_from(DocumentRevision)) == 3
        assert await session.scalar(select(func.count()).select_from(FormatIssue)) == 3
        assert await session.scalar(select(func.count()).select_from(Episode)) == 0


@pytest.mark.asyncio
async def test_document_import_is_concurrency_safe_and_cross_workspace_hidden(
    client: httpx.AsyncClient,
    session_factory: async_sessionmaker[AsyncSession],
) -> None:
    headers, _, project = await _project(
        client, email="whole-script-concurrent@example.com"
    )
    endpoint = f"/api/v1/projects/{project['id']}/script-imports"

    first, second = await asyncio.gather(
        client.post(endpoint, headers=headers, json=_text_payload()),
        client.post(endpoint, headers=headers, json=_text_payload()),
    )

    assert first.status_code == 201
    assert second.status_code == 201
    assert first.json()["data"] == second.json()["data"]
    revision_id = first.json()["data"]["revision"]["id"]

    stranger_headers, _, _ = await _project(
        client, email="whole-script-stranger@example.com"
    )
    hidden = await client.get(
        f"/api/v1/document-revisions/{revision_id}", headers=stranger_headers
    )
    assert hidden.status_code == 404

    async with session_factory() as session:
        assert await session.scalar(select(func.count()).select_from(ScriptDocument)) == 1
        assert await session.scalar(select(func.count()).select_from(DocumentRevision)) == 1


@pytest.mark.asyncio
async def test_ready_utf8_document_media_is_bounded_and_imported_by_fixed_version(
    app: FastAPI,
    client: httpx.AsyncClient,
    session_factory: async_sessionmaker[AsyncSession],
) -> None:
    storage = MemoryDocumentStorage()
    app.dependency_overrides[get_media_storage] = lambda: MediaStorage(
        port=storage,
        profile="memory",
        bucket="documents",
    )
    headers, workspace_id, project = await _project(
        client, email="whole-script-media@example.com"
    )
    body = "第一集\n上传正文。\n\n第二集\n继续。".encode()
    initialized = await client.post(
        "/api/v1/media/uploads",
        headers=headers,
        json={
            "workspace_id": workspace_id,
            "kind": "document",
            "filename": "whole-script.md",
            "size_bytes": len(body),
            "mime_type": "text/markdown",
            "sha256": hashlib.sha256(body).hexdigest(),
            "idempotency_key": "whole-script-media-upload",
        },
    )
    assert initialized.status_code == 201
    storage.objects[storage.upload_keys[-1]] = (body, "text/markdown")
    completed = await client.post(
        f"/api/v1/media/uploads/{initialized.json()['data']['upload_session']['id']}/complete",
        headers=headers,
        json={},
    )
    assert completed.status_code == 200
    media_version_id = completed.json()["data"]["version"]["id"]
    async with session_factory.begin() as session:
        version = await session.get(MediaVersion, UUID(media_version_id))
        assert version is not None
        version.probe_status = "ready"

    imported = await client.post(
        f"/api/v1/projects/{project['id']}/script-imports",
        headers=headers,
        json={
            "input_type": "media",
            "title": "上传整剧",
            "media_version_id": media_version_id,
            "language": "zh-CN",
            "rights_declaration": "确认拥有上传文本的使用权",
            "idempotency_key": "whole-script-media-import",
        },
    )

    assert imported.status_code == 201
    result = imported.json()["data"]
    assert result["document"]["source_type"] == "media"
    assert result["document"]["source_media_version_id"] == media_version_id
    assert result["revision"]["source_media_version_id"] == media_version_id
    assert result["revision"]["raw_text"] == body.decode()
    assert result["revision"]["analysis_status"] == "deterministic"
