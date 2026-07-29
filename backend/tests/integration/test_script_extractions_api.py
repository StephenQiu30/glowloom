import asyncio
from collections.abc import AsyncIterator
from typing import Any
from uuid import UUID

import httpx
import pytest
from pydantic import SecretStr, ValidationError
from sqlalchemy import func, select
from sqlalchemy.ext.asyncio import AsyncSession, async_sessionmaker

from app.core.config import Settings
from app.core.database import Base, create_engine, get_async_session, validate_test_database_url
from app.core.errors import ApiError, ErrorCode
from app.main import create_app
from app.modules.identity.policy import ActorContext
from app.modules.messaging.consumer import IO_SCRIPT_EXTRACTION_CONSUMER, consume_envelope
from app.modules.messaging.models import InboxDelivery, OutboxEvent
from app.modules.messaging.service import envelope_from_event
from app.modules.production import service as production_service
from app.modules.production.models import Task
from app.modules.production.schemas import ScriptExtractionTaskCommand
from app.modules.scripts import schemas as script_schemas
from app.modules.scripts import service as scripts_service

TEST_DATABASE_URL = validate_test_database_url(
    "postgresql+asyncpg://postgres@127.0.0.1:5432/lanverse_test",
    "postgresql+asyncpg://postgres@127.0.0.1:5432/lanverse",
)


@pytest.fixture
async def session_factory() -> AsyncIterator[async_sessionmaker[AsyncSession]]:
    engine = create_engine(TEST_DATABASE_URL)
    async with engine.begin() as connection:
        await connection.run_sync(Base.metadata.drop_all)
        await connection.run_sync(Base.metadata.create_all)
    factory = async_sessionmaker(engine, expire_on_commit=False)
    try:
        yield factory
    finally:
        async with engine.begin() as connection:
            await connection.run_sync(Base.metadata.drop_all)
        await engine.dispose()


@pytest.fixture
async def client(
    session_factory: async_sessionmaker[AsyncSession],
) -> AsyncIterator[httpx.AsyncClient]:
    async def _test_session() -> AsyncIterator[AsyncSession]:
        async with session_factory() as session:
            yield session

    app = create_app(
        Settings(
            environment="test",
            database_url=TEST_DATABASE_URL,
            jwt_secret_key=SecretStr("extraction-test-secret-with-at-least-32-bytes"),
        )
    )
    app.dependency_overrides[get_async_session] = _test_session
    async with httpx.AsyncClient(
        transport=httpx.ASGITransport(app=app), base_url="http://test"
    ) as test_client:
        yield test_client


async def _identity(
    client: httpx.AsyncClient,
    *,
    email: str,
) -> tuple[dict[str, str], str]:
    response = await client.post(
        "/api/v1/auth/register",
        json={
            "email": email,
            "password": "a-secure-extraction-password",
            "display_name": "提取负责人",
        },
    )
    assert response.status_code == 201
    data = response.json()["data"]
    return {"authorization": f"Bearer {data['access_token']}"}, data["workspace"]["id"]


async def _published_script(
    client: httpx.AsyncClient,
    headers: dict[str, str],
    workspace_id: str,
    *,
    import_key: str = "extraction-import-001",
) -> tuple[dict[str, Any], dict[str, Any], dict[str, Any]]:
    project_response = await client.post(
        "/api/v1/projects",
        headers=headers,
        json={
            "workspace_id": workspace_id,
            "name": "剧本提取项目",
            "aspect_ratio": "9:16",
            "language": "zh-CN",
            "target_duration_ms": 90000,
        },
    )
    assert project_response.status_code == 201
    project = project_response.json()["data"]
    episode_response = await client.post(
        f"/api/v1/projects/{project['id']}/episodes",
        headers=headers,
        json={"name": "提取单集", "target_duration_ms": 90000},
    )
    assert episode_response.status_code == 201
    episode = episode_response.json()["data"]
    imported_response = await client.post(
        f"/api/v1/episodes/{episode['id']}/script-sources",
        headers=headers,
        json={
            "input_type": "text",
            "title": "待提取剧本",
            "body": "第一场\n角色甲：开始。",
            "rights_declaration": "确认拥有测试文本使用权",
            "idempotency_key": import_key,
        },
    )
    assert imported_response.status_code == 201
    imported = imported_response.json()["data"]
    published_response = await client.post(
        f"/api/v1/script-sources/{imported['source']['id']}/versions",
        headers=headers,
        json={
            "body": "第一场\n角色甲：开始。\n角色乙：收到。",
            "expected_current_version_id": None,
        },
    )
    assert published_response.status_code == 201
    return episode, imported, published_response.json()["data"]["version"]


def _extraction_payload(
    *,
    idempotency_key: str = "extract-script-001",
) -> dict[str, str]:
    return {"scope": "full", "idempotency_key": idempotency_key}


@pytest.mark.asyncio
async def test_start_extraction_is_atomic_concurrent_and_body_free(
    client: httpx.AsyncClient,
    session_factory: async_sessionmaker[AsyncSession],
) -> None:
    headers, workspace_id = await _identity(
        client, email="extraction-owner@example.com"
    )
    episode, imported, published = await _published_script(
        client, headers, workspace_id
    )
    endpoint = f"/api/v1/script-versions/{published['id']}/extractions"

    first, second = await asyncio.gather(
        client.post(endpoint, headers=headers, json=_extraction_payload()),
        client.post(endpoint, headers=headers, json=_extraction_payload()),
    )
    assert first.status_code == 202
    assert second.status_code == 202
    assert first.json()["data"] == second.json()["data"]
    batch = first.json()["data"]
    assert batch["workspace_id"] == workspace_id
    assert batch["script_version_id"] == published["id"]
    assert batch["scope"] == "full"
    assert batch["extractor_version"] == "script-structure-v1"
    assert batch["input_hash"] == published["content_hash"]
    assert batch["status"] == "queued"
    assert batch["confirmed_script_version_id"] is None
    assert "idempotency_key" not in batch
    assert batch["task"]["request_id"] == batch["id"]
    assert batch["task"]["scope"]["episode_id"] == episode["id"]
    assert batch["task"]["scope"]["input_version_id"] == published["id"]
    assert batch["task"]["scope"]["input_hash"] == published["content_hash"]
    assert batch["task"]["status"] == "queued"

    fetched = await client.get(
        f"/api/v1/extraction-batches/{batch['id']}", headers=headers
    )
    assert fetched.status_code == 200
    assert fetched.json()["data"] == batch
    invalid_scope = await client.post(
        endpoint,
        headers=headers,
        json={"scope": "selection", "idempotency_key": "invalid-scope"},
    )
    assert invalid_scope.status_code == 422
    draft_rejected = await client.post(
        f"/api/v1/script-versions/{imported['version']['id']}/extractions",
        headers=headers,
        json=_extraction_payload(idempotency_key="draft-extraction"),
    )
    assert draft_rejected.status_code == 409
    assert draft_rejected.json()["error"]["code"] == "state_conflict"

    async with session_factory() as session:
        batch_table = Base.metadata.tables["scr_extraction_batches"]
        assert await session.scalar(select(func.count()).select_from(batch_table)) == 1
        assert await session.scalar(select(func.count()).select_from(Task)) == 1
        assert await session.scalar(select(func.count()).select_from(OutboxEvent)) == 1
        event = await session.scalar(select(OutboxEvent))
        assert event is not None
        assert event.event_type == "script_extraction.requested"
        assert event.aggregate_id == UUID(batch["task"]["id"])
        assert event.payload == {"task_id": batch["task"]["id"]}
        serialized_event = str(event.payload)
        assert published["body"] not in serialized_event
        assert published["content_hash"] not in serialized_event


@pytest.mark.asyncio
async def test_extraction_transaction_rolls_back_all_three_facts(
    client: httpx.AsyncClient,
    session_factory: async_sessionmaker[AsyncSession],
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    headers, workspace_id = await _identity(
        client, email="extraction-rollback@example.com"
    )
    _, _, published = await _published_script(
        client, headers, workspace_id, import_key="rollback-import"
    )
    real_create = production_service.create_script_extraction_task

    async def _fail_after_task_creation(
        session: AsyncSession,
        actor: ActorContext,
        command: ScriptExtractionTaskCommand,
        *,
        trace_id: str,
    ) -> Task:
        await real_create(session, actor, command, trace_id=trace_id)
        raise ApiError(
            ErrorCode.INTERNAL_ERROR,
            "Synthetic extraction rollback",
            status_code=500,
        )

    monkeypatch.setattr(
        production_service,
        "create_script_extraction_task",
        _fail_after_task_creation,
    )
    response = await client.post(
        f"/api/v1/script-versions/{published['id']}/extractions",
        headers=headers,
        json=_extraction_payload(idempotency_key="rollback-extraction"),
    )
    assert response.status_code == 500

    async with session_factory() as session:
        batch_table = Base.metadata.tables["scr_extraction_batches"]
        assert await session.scalar(select(func.count()).select_from(batch_table)) == 0
        assert await session.scalar(select(func.count()).select_from(Task)) == 0
        assert await session.scalar(select(func.count()).select_from(OutboxEvent)) == 0


@pytest.mark.asyncio
async def test_unconfigured_worker_updates_batch_and_task_once_and_is_private(
    client: httpx.AsyncClient,
    session_factory: async_sessionmaker[AsyncSession],
) -> None:
    headers, workspace_id = await _identity(
        client, email="extraction-worker-owner@example.com"
    )
    _, _, published = await _published_script(
        client, headers, workspace_id, import_key="worker-import"
    )
    started = await client.post(
        f"/api/v1/script-versions/{published['id']}/extractions",
        headers=headers,
        json=_extraction_payload(idempotency_key="worker-extraction"),
    )
    assert started.status_code == 202
    batch = started.json()["data"]

    async with session_factory() as session:
        event = await session.scalar(
            select(OutboxEvent).where(
                OutboxEvent.aggregate_id == batch["task"]["id"]
            )
        )
        assert event is not None
        session.expunge(event)
    envelope = envelope_from_event(event)
    async with session_factory() as session:
        async with session.begin():
            assert (
                await consume_envelope(
                    session,
                    envelope,
                    consumer_name=IO_SCRIPT_EXTRACTION_CONSUMER,
                )
                == "completed"
            )
    async with session_factory() as session:
        async with session.begin():
            assert (
                await consume_envelope(
                    session,
                    envelope,
                    consumer_name=IO_SCRIPT_EXTRACTION_CONSUMER,
                )
                == "duplicate"
            )

    fetched = await client.get(
        f"/api/v1/extraction-batches/{batch['id']}", headers=headers
    )
    assert fetched.status_code == 200
    failed = fetched.json()["data"]
    assert failed["status"] == "failed"
    assert failed["task"]["status"] == "failed"
    assert failed["task"]["error"] == {
        "code": "ai_service_unavailable",
        "retryable": False,
        "summary": "AI extraction service is not configured",
    }
    assert failed["task"]["next_action"] == "configure_ai_service"

    stranger_headers, _ = await _identity(
        client, email="extraction-worker-stranger@example.com"
    )
    hidden = await client.get(
        f"/api/v1/extraction-batches/{batch['id']}", headers=stranger_headers
    )
    assert hidden.status_code == 404
    async with session_factory() as session:
        assert await session.scalar(select(func.count()).select_from(InboxDelivery)) == 1
        task = await session.get(Task, batch["task"]["id"])
        assert task is not None
        assert task.revision == 2


def _typed_extraction_result() -> dict[str, Any]:
    return {
        "candidates": [
            {
                "candidate_key": "scene-001",
                "source_range": {"start": 0, "end": 3},
                "proposal": {
                    "kind": "scene",
                    "heading": "第一场",
                    "location": "室内",
                    "time_of_day": "白天",
                    "summary": "两人确认行动",
                },
                "confidence_note": "场次标题清晰",
            },
            {
                "candidate_key": "dialogue-001",
                "source_range": {"start": 4, "end": 10},
                "proposal": {
                    "kind": "dialogue",
                    "scene_candidate_key": "scene-001",
                    "speaker_candidate": "角色甲",
                    "dialogue_kind": "spoken",
                    "text": "开始。",
                    "performance_note": "坚定",
                },
                "confidence_note": None,
            },
            {
                "candidate_key": "asset-001",
                "source_range": {"start": 4, "end": 7},
                "proposal": {
                    "kind": "asset",
                    "asset_kind": "character",
                    "name": "角色甲",
                    "description": "行动发起者",
                },
                "confidence_note": "说话主体可形成角色资产",
            },
            {
                "candidate_key": "shot-001",
                "source_range": {"start": 0, "end": 15},
                "proposal": {
                    "kind": "shot",
                    "scene_candidate_key": "scene-001",
                    "title": "角色甲开场",
                    "purpose": "交代行动开始",
                },
                "confidence_note": None,
            },
            {
                "candidate_key": "continuity-001",
                "source_range": {"start": 11, "end": 19},
                "proposal": {
                    "kind": "continuity",
                    "severity": "warning",
                    "issue": "角色乙首次出现",
                    "suggestion": "确认角色设定",
                },
                "confidence_note": "仅作为人工检查提示",
            },
        ]
    }


@pytest.mark.asyncio
async def test_extraction_candidates_are_typed_idempotent_paginated_and_private(
    client: httpx.AsyncClient,
    session_factory: async_sessionmaker[AsyncSession],
) -> None:
    headers, workspace_id = await _identity(
        client, email="candidate-owner@example.com"
    )
    _, _, published = await _published_script(
        client, headers, workspace_id, import_key="candidate-import"
    )
    started = await client.post(
        f"/api/v1/script-versions/{published['id']}/extractions",
        headers=headers,
        json=_extraction_payload(idempotency_key="candidate-extraction"),
    )
    assert started.status_code == 202
    batch = started.json()["data"]

    result_model = script_schemas.__dict__["ScriptExtractionResult"]
    record_result = scripts_service.__dict__["record_extraction_result"]
    result = result_model.model_validate(_typed_extraction_result())
    with pytest.raises(ValidationError):
        result_model.model_validate(
            {
                "candidates": [
                    {
                        "candidate_key": "wrong-union",
                        "source_range": {"start": 0, "end": 3},
                        "proposal": {
                            "kind": "asset",
                            "asset_kind": "character",
                            "name": "角色甲",
                            "description": "角色",
                            "speaker_candidate": "不允许的跨类型字段",
                        },
                    }
                ]
            }
        )

    for _ in range(2):
        async with session_factory() as session:
            async with session.begin():
                await record_result(session, UUID(batch["id"]), result)

    listed = await client.get(
        f"/api/v1/extraction-batches/{batch['id']}/candidates",
        headers=headers,
    )
    assert listed.status_code == 200
    page = listed.json()["data"]
    assert page["total"] == 5
    assert page["limit"] == 20
    assert page["offset"] == 0
    assert [item["proposal"]["kind"] for item in page["items"]] == [
        "scene",
        "shot",
        "asset",
        "dialogue",
        "continuity",
    ]
    scene = page["items"][0]
    assert scene["candidate_key"] == "scene-001"
    assert scene["source_range"] == {"start": 0, "end": 3}
    assert scene["status"] == "pending"
    assert scene["revision"] == 1
    assert scene["required"] is True
    assert "workspace_id" not in scene

    filtered = await client.get(
        f"/api/v1/extraction-batches/{batch['id']}/candidates",
        headers=headers,
        params={"kind": "scene", "status": "pending", "limit": 1},
    )
    assert filtered.status_code == 200
    assert filtered.json()["data"]["items"] == [scene]
    detail = await client.get(
        f"/api/v1/extraction-candidates/{scene['id']}", headers=headers
    )
    assert detail.status_code == 200
    assert detail.json()["data"] == scene
    invalid_filter = await client.get(
        f"/api/v1/extraction-batches/{batch['id']}/candidates",
        headers=headers,
        params={"kind": "unknown-kind"},
    )
    assert invalid_filter.status_code == 422

    completed = await client.get(
        f"/api/v1/extraction-batches/{batch['id']}", headers=headers
    )
    assert completed.status_code == 200
    completed_batch = completed.json()["data"]
    assert completed_batch["status"] == "succeeded"
    assert completed_batch["candidate_count"] == 5
    assert completed_batch["task"]["status"] == "succeeded"
    assert completed_batch["task"]["progress_stage"] == "completed"
    assert completed_batch["task"]["next_action"] == "review_candidates"
    assert completed_batch["task"]["revision"] == 2

    stranger_headers, _ = await _identity(
        client, email="candidate-stranger@example.com"
    )
    hidden_list = await client.get(
        f"/api/v1/extraction-batches/{batch['id']}/candidates",
        headers=stranger_headers,
    )
    hidden_detail = await client.get(
        f"/api/v1/extraction-candidates/{scene['id']}", headers=stranger_headers
    )
    assert hidden_list.status_code == 404
    assert hidden_detail.status_code == 404

    async with session_factory() as session:
        candidate_table = Base.metadata.tables["scr_extraction_candidates"]
        assert (
            await session.scalar(select(func.count()).select_from(candidate_table))
            == 5
        )


@pytest.mark.asyncio
async def test_extraction_result_rejects_invalid_ranges_and_changed_replay(
    client: httpx.AsyncClient,
    session_factory: async_sessionmaker[AsyncSession],
) -> None:
    headers, workspace_id = await _identity(
        client, email="candidate-conflict@example.com"
    )
    _, _, published = await _published_script(
        client, headers, workspace_id, import_key="candidate-conflict-import"
    )
    started = await client.post(
        f"/api/v1/script-versions/{published['id']}/extractions",
        headers=headers,
        json=_extraction_payload(idempotency_key="candidate-conflict-extraction"),
    )
    assert started.status_code == 202
    batch = started.json()["data"]
    result_model = script_schemas.__dict__["ScriptExtractionResult"]
    record_result = scripts_service.__dict__["record_extraction_result"]

    invalid = _typed_extraction_result()
    invalid["candidates"][0]["source_range"] = {"start": 0, "end": 99_999}
    async with session_factory() as session:
        with pytest.raises(ApiError) as invalid_error:
            async with session.begin():
                await record_result(
                    session,
                    UUID(batch["id"]),
                    result_model.model_validate(invalid),
                )
        assert invalid_error.value.code == ErrorCode.INVALID_REQUEST

    valid = result_model.model_validate(_typed_extraction_result())
    async with session_factory() as session:
        async with session.begin():
            await record_result(session, UUID(batch["id"]), valid)

    changed = _typed_extraction_result()
    changed["candidates"][0]["proposal"]["summary"] = "不同的重放结果"
    async with session_factory() as session:
        with pytest.raises(ApiError) as conflict_error:
            async with session.begin():
                await record_result(
                    session,
                    UUID(batch["id"]),
                    result_model.model_validate(changed),
                )
        assert conflict_error.value.code == ErrorCode.RESOURCE_CONFLICT

    async with session_factory() as session:
        candidate_table = Base.metadata.tables["scr_extraction_candidates"]
        assert (
            await session.scalar(select(func.count()).select_from(candidate_table))
            == 5
        )
