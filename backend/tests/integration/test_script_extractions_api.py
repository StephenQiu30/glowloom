import asyncio
from typing import Any
from uuid import UUID

import httpx
import pytest
from pydantic import ValidationError
from sqlalchemy import func, select
from sqlalchemy.ext.asyncio import AsyncSession, async_sessionmaker
from uuid6 import uuid7

from app import io_worker
from app.core.database import Base
from app.core.errors import ApiError, ErrorCode
from app.modules.governance.audit.models import AuditEvent
from app.modules.identity import ActorContext
from app.modules.messaging.consumer import IO_SCRIPT_EXTRACTION_CONSUMER, consume_envelope
from app.modules.messaging.models import InboxDelivery, OutboxEvent
from app.modules.messaging.service import envelope_from_event
from app.modules.production import ScriptExtractionTaskCommand, TaskResponse
from app.modules.production.models import Task
from app.modules.scripts.extractions import schemas as script_schemas
from app.modules.scripts.extractions import service as scripts_service
from tests.support.identity_builders import register_identity_response


async def _identity(
    client: httpx.AsyncClient,
    *,
    email: str,
) -> tuple[dict[str, str], str]:
    response = await register_identity_response(
        client,
        email=email,
        password="a-secure-extraction-password",
        display_name="提取负责人",
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
    assert batch["extractor_version"] == (
        "deepseek-v4-pro:thinking-off:lc-deepseek-1.1.0:prompt-v1:schema-v1"
    )
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
    real_create = scripts_service.create_script_extraction_task

    async def _fail_after_task_creation(
        session: AsyncSession,
        actor: ActorContext,
        command: ScriptExtractionTaskCommand,
        *,
        trace_id: str,
    ) -> TaskResponse:
        await real_create(session, actor, command, trace_id=trace_id)
        raise ApiError(
            ErrorCode.INTERNAL_ERROR,
            "Synthetic extraction rollback",
            status_code=500,
        )

    monkeypatch.setattr(
        scripts_service,
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


class _RecordingExtractionMessage:
    def __init__(self, body: bytes) -> None:
        self.body = body
        self.ack_count = 0
        self.nack_requeues: list[bool] = []

    async def ack(self) -> None:
        self.ack_count += 1

    async def nack(self, *, requeue: bool) -> None:
        self.nack_requeues.append(requeue)


class _RecordingScriptExtractor:
    def __init__(self) -> None:
        self.inputs: list[str] = []

    async def extract(self, script_body: str) -> script_schemas.ScriptExtractionResult:
        self.inputs.append(script_body)
        return script_schemas.ScriptExtractionResult.model_validate(
            _typed_extraction_result()
        )


@pytest.mark.asyncio
async def test_configured_worker_records_real_adapter_result_once(
    client: httpx.AsyncClient,
    session_factory: async_sessionmaker[AsyncSession],
) -> None:
    headers, workspace_id = await _identity(
        client, email="extraction-configured-worker@example.com"
    )
    _, _, published = await _published_script(
        client,
        headers,
        workspace_id,
        import_key="configured-worker-import",
    )
    started = await client.post(
        f"/api/v1/script-versions/{published['id']}/extractions",
        headers=headers,
        json=_extraction_payload(idempotency_key="configured-worker-extraction"),
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
        body = envelope_from_event(event).model_dump_json().encode()

    extractor = _RecordingScriptExtractor()
    first_message = _RecordingExtractionMessage(body)
    first_result = await io_worker.process_incoming_message(
        first_message,
        session_factory,
        extractor=extractor,
    )

    assert first_result == "completed"
    assert first_message.ack_count == 1
    assert first_message.nack_requeues == []
    assert extractor.inputs == [published["body"]]

    fetched = await client.get(
        f"/api/v1/extraction-batches/{batch['id']}", headers=headers
    )
    assert fetched.status_code == 200
    completed = fetched.json()["data"]
    assert completed["status"] == "succeeded"
    assert completed["candidate_count"] == 5
    assert completed["task"]["status"] == "succeeded"
    assert completed["task"]["next_action"] == "review_candidates"

    duplicate_message = _RecordingExtractionMessage(body)
    duplicate_result = await io_worker.process_incoming_message(
        duplicate_message,
        session_factory,
        extractor=extractor,
    )

    assert duplicate_result == "duplicate"
    assert duplicate_message.ack_count == 1
    assert duplicate_message.nack_requeues == []
    assert extractor.inputs == [published["body"]]
    async with session_factory() as session:
        inbox = await session.scalar(select(InboxDelivery))
        assert inbox is not None
        assert inbox.status == "completed"
        assert inbox.attempt_count == 2
        audit_events = list(
            await session.scalars(
                select(AuditEvent)
                .where(
                    AuditEvent.target_type == "task",
                    AuditEvent.target_id == UUID(batch["task"]["id"]),
                )
                .order_by(AuditEvent.occurred_at, AuditEvent.id)
            )
        )
        assert [item.action for item in audit_events] == [
            "task.created",
            "task.started",
            "task.succeeded",
        ]
        assert {item.trace_id for item in audit_events[1:]} == {event.trace_id}
        assert audit_events[1].event_metadata["previous_status"] == "queued"
        assert audit_events[1].event_metadata["status"] == "running"
        assert audit_events[2].event_metadata["previous_status"] == "running"
        assert audit_events[2].event_metadata["status"] == "succeeded"


@pytest.mark.asyncio
async def test_worker_redelivery_after_result_commit_failure_does_not_call_provider_again(
    client: httpx.AsyncClient,
    session_factory: async_sessionmaker[AsyncSession],
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    headers, workspace_id = await _identity(
        client, email="extraction-unknown-worker@example.com"
    )
    _, _, published = await _published_script(
        client,
        headers,
        workspace_id,
        import_key="unknown-worker-import",
    )
    started = await client.post(
        f"/api/v1/script-versions/{published['id']}/extractions",
        headers=headers,
        json=_extraction_payload(idempotency_key="unknown-worker-extraction"),
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
        body = envelope_from_event(event).model_dump_json().encode()

    extractor = _RecordingScriptExtractor()
    real_finalize = io_worker.finalize_extraction_success

    async def fail_result_commit(*_: object, **__: object) -> None:
        raise RuntimeError("synthetic result commit failure")

    monkeypatch.setattr(io_worker, "finalize_extraction_success", fail_result_commit)
    first_message = _RecordingExtractionMessage(body)
    first_result = await io_worker.process_incoming_message(
        first_message,
        session_factory,
        extractor=extractor,
    )

    assert first_result == "requeued"
    assert first_message.ack_count == 0
    assert first_message.nack_requeues == [True]
    assert extractor.inputs == [published["body"]]

    monkeypatch.setattr(io_worker, "finalize_extraction_success", real_finalize)
    redelivery = _RecordingExtractionMessage(body)
    redelivery_result = await io_worker.process_incoming_message(
        redelivery,
        session_factory,
        extractor=extractor,
    )

    assert redelivery_result == "completed"
    assert redelivery.ack_count == 1
    assert redelivery.nack_requeues == []
    assert extractor.inputs == [published["body"]]
    fetched = await client.get(
        f"/api/v1/extraction-batches/{batch['id']}", headers=headers
    )
    assert fetched.status_code == 200
    unknown = fetched.json()["data"]
    assert unknown["status"] == "unknown"
    assert unknown["task"]["status"] == "unknown"
    assert unknown["task"]["error"] == {
        "code": "ai_result_unknown",
        "retryable": False,
        "summary": "DeepSeek response outcome is unknown",
    }
    assert unknown["task"]["next_action"] == "start_new_extraction"
    async with session_factory() as session:
        audit_events = list(
            await session.scalars(
                select(AuditEvent)
                .where(
                    AuditEvent.target_type == "task",
                    AuditEvent.target_id == UUID(batch["task"]["id"]),
                )
                .order_by(AuditEvent.occurred_at, AuditEvent.id)
            )
        )
        assert [item.action for item in audit_events] == [
            "task.created",
            "task.started",
            "task.unknown",
        ]
        assert audit_events[-1].trace_id == event.trace_id
        assert audit_events[-1].event_metadata["previous_status"] == "running"
        assert audit_events[-1].event_metadata["error_code"] == "ai_result_unknown"
        assert "DeepSeek response outcome is unknown" not in str(
            audit_events[-1].event_metadata
        )


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
                await record_result(
                    session,
                    UUID(batch["id"]),
                    result,
                    trace_id="candidate-result-replay",
                )

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
        assert (
            await session.scalar(
                select(func.count())
                .select_from(AuditEvent)
                .where(
                    AuditEvent.target_id == UUID(batch["task"]["id"]),
                    AuditEvent.action == "task.succeeded",
                )
            )
            == 1
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
                    trace_id="candidate-invalid-range",
                )
        assert invalid_error.value.code == ErrorCode.INVALID_REQUEST

    valid = result_model.model_validate(_typed_extraction_result())
    async with session_factory() as session:
        async with session.begin():
            await record_result(
                session,
                UUID(batch["id"]),
                valid,
                trace_id="candidate-valid-result",
            )

    changed = _typed_extraction_result()
    changed["candidates"][0]["proposal"]["summary"] = "不同的重放结果"
    async with session_factory() as session:
        with pytest.raises(ApiError) as conflict_error:
            async with session.begin():
                await record_result(
                    session,
                    UUID(batch["id"]),
                    result_model.model_validate(changed),
                    trace_id="candidate-changed-replay",
                )
        assert conflict_error.value.code == ErrorCode.RESOURCE_CONFLICT

    async with session_factory() as session:
        candidate_table = Base.metadata.tables["scr_extraction_candidates"]
        assert (
            await session.scalar(select(func.count()).select_from(candidate_table))
            == 5
        )


async def _completed_candidate_batch(
    client: httpx.AsyncClient,
    session_factory: async_sessionmaker[AsyncSession],
    headers: dict[str, str],
    workspace_id: str,
    *,
    key: str,
    second_asset: bool = False,
) -> tuple[dict[str, Any], list[dict[str, Any]]]:
    _, _, published = await _published_script(
        client,
        headers,
        workspace_id,
        import_key=f"{key}-import",
    )
    started = await client.post(
        f"/api/v1/script-versions/{published['id']}/extractions",
        headers=headers,
        json=_extraction_payload(idempotency_key=f"{key}-extraction"),
    )
    assert started.status_code == 202
    batch = started.json()["data"]
    raw_result = _typed_extraction_result()
    if second_asset:
        raw_result["candidates"].append(
            {
                "candidate_key": "asset-002",
                "source_range": {"start": 12, "end": 15},
                "proposal": {
                    "kind": "asset",
                    "asset_kind": "character",
                    "name": "角色乙",
                    "description": "回应行动的角色",
                },
                "confidence_note": None,
            }
        )
    result = script_schemas.ScriptExtractionResult.model_validate(raw_result)
    async with session_factory() as session:
        async with session.begin():
            await scripts_service.record_extraction_result(
                session,
                UUID(batch["id"]),
                result,
                trace_id=f"fixture-result-{key}",
            )
    listed = await client.get(
        f"/api/v1/extraction-batches/{batch['id']}/candidates",
        headers=headers,
        params={"limit": 100},
    )
    assert listed.status_code == 200
    return batch, listed.json()["data"]["items"]


@pytest.mark.asyncio
async def test_candidate_decision_is_append_only_idempotent_and_refreshable(
    client: httpx.AsyncClient,
    session_factory: async_sessionmaker[AsyncSession],
) -> None:
    headers, workspace_id = await _identity(
        client, email="decision-owner@example.com"
    )
    _, candidates = await _completed_candidate_batch(
        client,
        session_factory,
        headers,
        workspace_id,
        key="decision-idempotent",
    )
    scene = next(item for item in candidates if item["kind"] == "scene")
    endpoint = f"/api/v1/extraction-candidates/{scene['id']}/decisions"
    payload = {
        "decision_key": "accept-scene-001",
        "expected_revision": 1,
        "decision": {"action": "accept_new"},
    }

    first, second = await asyncio.gather(
        client.post(endpoint, headers=headers, json=payload),
        client.post(endpoint, headers=headers, json=payload),
    )
    assert first.status_code == 201
    assert second.status_code == 201
    assert first.json()["data"] == second.json()["data"]
    result = first.json()["data"]
    assert result["candidate"]["status"] == "accepted"
    assert result["candidate"]["revision"] == 2
    evidence = result["evidence"]
    assert evidence["candidate_id"] == scene["id"]
    assert evidence["sequence"] == 1
    assert evidence["decision_key"] == "accept-scene-001"
    assert evidence["decision"] == {"action": "accept_new"}
    assert evidence["actor_id"]

    replay = await client.post(endpoint, headers=headers, json=payload)
    assert replay.status_code == 201
    assert replay.json()["data"] == result
    changed_replay = await client.post(
        endpoint,
        headers=headers,
        json={
            **payload,
            "decision": {"action": "ignore"},
        },
    )
    assert changed_replay.status_code == 409
    assert changed_replay.json()["error"]["code"] == "resource_conflict"
    stale = await client.post(
        endpoint,
        headers=headers,
        json={
            "decision_key": "stale-scene-decision",
            "expected_revision": 1,
            "decision": {"action": "ignore"},
        },
    )
    assert stale.status_code == 409
    assert stale.json()["error"]["code"] == "version_conflict"
    assert stale.json()["error"]["details"] == {"current_revision": 2}

    history = await client.get(endpoint, headers=headers)
    assert history.status_code == 200
    history_page = history.json()["data"]
    assert history_page["total"] == 1
    assert history_page["items"] == [evidence]
    refreshed = await client.get(
        f"/api/v1/extraction-candidates/{scene['id']}", headers=headers
    )
    assert refreshed.status_code == 200
    assert refreshed.json()["data"] == result["candidate"]

    stranger_headers, _ = await _identity(
        client, email="decision-stranger@example.com"
    )
    hidden_write = await client.post(endpoint, headers=stranger_headers, json=payload)
    hidden_history = await client.get(endpoint, headers=stranger_headers)
    assert hidden_write.status_code == 404
    assert hidden_history.status_code == 404

    async with session_factory() as session:
        decision_table = Base.metadata.tables["scr_candidate_decisions"]
        assert (
            await session.scalar(select(func.count()).select_from(decision_table))
            == 1
        )


@pytest.mark.asyncio
async def test_candidate_decisions_validate_changes_merge_ignore_and_scope(
    client: httpx.AsyncClient,
    session_factory: async_sessionmaker[AsyncSession],
) -> None:
    headers, workspace_id = await _identity(
        client, email="decision-validation@example.com"
    )
    _, candidates = await _completed_candidate_batch(
        client,
        session_factory,
        headers,
        workspace_id,
        key="decision-validation",
        second_asset=True,
    )
    scene = next(item for item in candidates if item["kind"] == "scene")
    dialogue = next(item for item in candidates if item["kind"] == "dialogue")
    assets = [item for item in candidates if item["kind"] == "asset"]
    continuity = next(item for item in candidates if item["kind"] == "continuity")

    wrong_change = await client.post(
        f"/api/v1/extraction-candidates/{scene['id']}/decisions",
        headers=headers,
        json={
            "decision_key": "wrong-scene-change",
            "expected_revision": 1,
            "decision": {
                "action": "accept_with_changes",
                "proposal": dialogue["proposal"],
            },
        },
    )
    assert wrong_change.status_code == 422
    assert wrong_change.json()["error"]["code"] == "invalid_request"

    changed_proposal = {
        **scene["proposal"],
        "summary": "人工修订后的场景摘要",
    }
    accepted_change = await client.post(
        f"/api/v1/extraction-candidates/{scene['id']}/decisions",
        headers=headers,
        json={
            "decision_key": "edit-scene-001",
            "expected_revision": 1,
            "decision": {
                "action": "accept_with_changes",
                "proposal": changed_proposal,
            },
        },
    )
    assert accepted_change.status_code == 201
    assert accepted_change.json()["data"]["evidence"]["decision"] == {
        "action": "accept_with_changes",
        "proposal": changed_proposal,
    }

    cross_type_merge = await client.post(
        f"/api/v1/extraction-candidates/{assets[0]['id']}/decisions",
        headers=headers,
        json={
            "decision_key": "cross-kind-merge",
            "expected_revision": 1,
            "decision": {
                "action": "merge_into",
                "target_candidate_id": scene["id"],
            },
        },
    )
    assert cross_type_merge.status_code == 422
    assert cross_type_merge.json()["error"]["code"] == "invalid_request"
    merged = await client.post(
        f"/api/v1/extraction-candidates/{assets[0]['id']}/decisions",
        headers=headers,
        json={
            "decision_key": "merge-assets-001",
            "expected_revision": 1,
            "decision": {
                "action": "merge_into",
                "target_candidate_id": assets[1]["id"],
            },
        },
    )
    assert merged.status_code == 201
    assert merged.json()["data"]["candidate"]["status"] == "merged"

    ignored = await client.post(
        f"/api/v1/extraction-candidates/{continuity['id']}/decisions",
        headers=headers,
        json={
            "decision_key": "ignore-continuity-001",
            "expected_revision": 1,
            "decision": {"action": "ignore"},
        },
    )
    assert ignored.status_code == 201
    assert ignored.json()["data"]["candidate"]["status"] == "ignored"

    unsupported_link = await client.post(
        f"/api/v1/extraction-candidates/{assets[1]['id']}/decisions",
        headers=headers,
        json={
            "decision_key": "link-before-assets-module",
            "expected_revision": 1,
            "decision": {
                "action": "link_existing",
                "downstream_id": str(uuid7()),
            },
        },
    )
    assert unsupported_link.status_code == 409
    assert unsupported_link.json()["error"]["next_action"] == "confirm_structure"

    async with session_factory() as session:
        decision_table = Base.metadata.tables["scr_candidate_decisions"]
        assert (
            await session.scalar(select(func.count()).select_from(decision_table))
            == 3
        )
