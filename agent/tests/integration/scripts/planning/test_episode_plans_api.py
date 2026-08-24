import asyncio
import hashlib
import json
from copy import deepcopy
from pathlib import Path
from typing import Any
from uuid import UUID

import httpx
import pytest
from sqlalchemy import func, select
from sqlalchemy.ext.asyncio import AsyncSession, async_sessionmaker

from app.modules.messaging import envelope_from_event
from app.modules.messaging.contracts import MessageEnvelope
from app.modules.messaging.models import InboxDelivery, OutboxEvent
from app.modules.messaging.planning_consumer import (
    PreparedEpisodePlanning,
    prepare_episode_planning,
)
from app.modules.production.models import Task
from app.modules.projects.models import Episode
from app.modules.scripts.models import (
    EpisodePlan,
    EpisodeProposal,
    EpisodeSegmentOrigin,
    ExtractionBatch,
    ImportCommit,
    ScriptSource,
    ScriptVersion,
)
from app.modules.scripts.narratives.models import (
    NarrativeImpactAssessment,
    NarrativeStructure,
)
from app.modules.scripts.planning.schemas import (
    EpisodePlanningProviderProposal,
    EpisodePlanningProviderResult,
)
from app.modules.scripts.production_bibles.models import ProductionBible
from app.runtime.workers import io as io_worker
from tests.support.production_bibles import seed_confirmed_production_bible
from tests.support.project_builders import project_payload, register_project_owner

FIXTURE = Path(__file__).parents[3] / "fixtures/mvp_a/golden_candidate_harbor_countdown.json"


def _fixture_text() -> str:
    raw = json.loads(FIXTURE.read_text(encoding="utf-8"))
    return str(raw["full_script"])


async def _project_and_document(
    client: httpx.AsyncClient,
    *,
    email: str,
    text: str | None = None,
) -> tuple[dict[str, str], dict[str, Any], dict[str, Any]]:
    headers, workspace_id = await register_project_owner(client, email=email)
    project_response = await client.post(
        "/api/v1/projects",
        headers=headers,
        json=project_payload(workspace_id, name="分集计划验收项目"),
    )
    assert project_response.status_code == 201
    project = project_response.json()["data"]
    document_response = await client.post(
        f"/api/v1/projects/{project['id']}/script-imports",
        headers=headers,
        json={
            "input_type": "text",
            "title": "原创整剧原稿",
            "text": text if text is not None else _fixture_text(),
            "language": "zh-CN",
            "rights_declaration": "确认拥有该原创测试文本的使用权",
            "idempotency_key": f"document:{email}",
        },
    )
    assert document_response.status_code == 201
    return headers, project, document_response.json()["data"]


async def _explicit_plan(
    client: httpx.AsyncClient,
    headers: dict[str, str],
    revision_id: str,
    *,
    idempotency_key: str,
) -> httpx.Response:
    return await client.post(
        f"/api/v1/document-revisions/{revision_id}/episode-plans",
        headers=headers,
        json={
            "strategy": "explicit_markers",
            "target_duration_ms": 90_000,
            "requested_episode_count": None,
            "idempotency_key": idempotency_key,
        },
    )


@pytest.mark.asyncio
async def test_explicit_plan_is_reviewable_idempotent_and_writes_no_episode(
    client: httpx.AsyncClient,
    session_factory: async_sessionmaker[AsyncSession],
) -> None:
    headers, project, document = await _project_and_document(
        client,
        email="episode-plan-explicit@example.com",
    )

    created = await _explicit_plan(
        client,
        headers,
        document["revision"]["id"],
        idempotency_key="explicit-plan-001",
    )

    assert created.status_code == 201
    result = created.json()["data"]
    plan = result["plan"]
    proposals = result["proposals"]
    assert plan["document_revision_id"] == document["revision"]["id"]
    assert plan["project_id"] == project["id"]
    assert plan["strategy"] == "explicit_markers"
    assert plan["status"] == "review_ready"
    assert plan["revision"] == 1
    assert plan["target_duration_ms"] == 90_000
    assert plan["planning_task_id"] is None
    assert len(proposals) == 5
    assert [proposal["position"] for proposal in proposals] == [1, 2, 3, 4, 5]
    assert [proposal["title"] for proposal in proposals] == [
        "警报前夜",
        "被删掉的检修记录",
        "九十秒手动开闸",
        "替罪签名",
        "公开日志",
    ]
    assert proposals[0]["source_start"] == 0
    assert proposals[-1]["source_end"] == len(document["revision"]["normalized_text"])
    assert all(proposal["estimated_duration_ms"] > 0 for proposal in proposals)
    assert all(proposal["confidence"] == 1.0 for proposal in proposals)
    assert all(proposal["reason"] for proposal in proposals)
    assert all(proposal["boundary_evidence"]["kind"] == "explicit_marker" for proposal in proposals)
    assert "body" not in str(result["impact"])
    assert result["impact"] == {
        "active_episode_count": 0,
        "active_order_hash": hashlib.sha256(b"[]").hexdigest(),
        "allowed": True,
        "blockers": [],
        "project_revision": 1,
        "projected_episode_count": 5,
    }
    source = document["revision"]["normalized_text"]
    assert (
        "".join(source[proposal["source_start"] : proposal["source_end"]] for proposal in proposals)
        == source
    )
    assert all(
        proposals[index - 1]["source_end"] == proposals[index]["source_start"]
        for index in range(1, len(proposals))
    )

    repeated = await _explicit_plan(
        client,
        headers,
        document["revision"]["id"],
        idempotency_key="explicit-plan-001",
    )
    assert repeated.status_code == 201
    assert repeated.json()["data"] == result

    conflicting = await client.post(
        f"/api/v1/document-revisions/{document['revision']['id']}/episode-plans",
        headers=headers,
        json={
            "strategy": "explicit_markers",
            "target_duration_ms": 60_000,
            "requested_episode_count": None,
            "idempotency_key": "explicit-plan-001",
        },
    )
    assert conflicting.status_code == 409
    assert conflicting.json()["error"]["code"] == "resource_conflict"

    fetched = await client.get(f"/api/v1/episode-plans/{plan['id']}", headers=headers)
    assert fetched.status_code == 200
    assert fetched.json()["data"] == result

    async with session_factory() as session:
        assert await session.scalar(select(func.count()).select_from(EpisodePlan)) == 1
        assert await session.scalar(select(func.count()).select_from(EpisodeProposal)) == 5
        assert await session.scalar(select(func.count()).select_from(Episode)) == 0
        assert await session.scalar(select(func.count()).select_from(ScriptSource)) == 0
        assert await session.scalar(select(func.count()).select_from(ScriptVersion)) == 0


@pytest.mark.asyncio
async def test_boundaries_split_merge_and_titles_change_only_the_plan(
    client: httpx.AsyncClient,
    session_factory: async_sessionmaker[AsyncSession],
) -> None:
    text = (
        "第一集\n《起点》\n场景1：控制室，夜\n甲：开始。\n动作一。\n"
        "第二集\n《追击》\n场景2：港口，日\n乙：继续。\n动作二。"
    )
    headers, _, document = await _project_and_document(
        client,
        email="episode-plan-edit@example.com",
        text=text,
    )
    created = await _explicit_plan(
        client,
        headers,
        document["revision"]["id"],
        idempotency_key="editable-plan",
    )
    result = created.json()["data"]
    first, second = result["proposals"]
    original_boundary = second["source_start"]
    second_marker_end = next(
        block["source_end"]
        for block in document["blocks"]
        if block["source_start"] == original_boundary
    )

    invalid = await client.post(
        f"/api/v1/episode-plans/{result['plan']['id']}/move-boundary",
        headers=headers,
        json={
            "left_proposal_id": first["id"],
            "source_offset": second_marker_end - 1,
            "expected_revision": 1,
            "idempotency_key": "move-inside-block",
        },
    )
    assert invalid.status_code == 422
    assert invalid.json()["error"]["code"] == "validation_failed"
    assert invalid.json()["error"]["next_action"] == "choose_block_boundary"

    moved = await client.post(
        f"/api/v1/episode-plans/{result['plan']['id']}/move-boundary",
        headers=headers,
        json={
            "left_proposal_id": first["id"],
            "source_offset": second_marker_end,
            "expected_revision": 1,
            "idempotency_key": "move-valid",
        },
    )
    assert moved.status_code == 200
    moved_result = moved.json()["data"]
    assert moved_result["plan"]["revision"] == 2
    assert moved_result["proposals"][0]["source_end"] == second_marker_end
    assert moved_result["proposals"][1]["source_start"] == second_marker_end

    split_offset = document["blocks"][-1]["source_start"]
    split = await client.post(
        f"/api/v1/episode-plans/{result['plan']['id']}/split",
        headers=headers,
        json={
            "proposal_id": second["id"],
            "source_offset": split_offset,
            "new_title": "动作尾声",
            "expected_revision": 2,
            "idempotency_key": "split-valid",
        },
    )
    assert split.status_code == 200
    split_result = split.json()["data"]
    assert split_result["plan"]["revision"] == 3
    assert [item["position"] for item in split_result["proposals"]] == [1, 2, 3]
    assert split_result["proposals"][2]["title"] == "动作尾声"

    renamed = await client.post(
        f"/api/v1/episode-plans/{result['plan']['id']}/rename",
        headers=headers,
        json={
            "proposal_id": split_result["proposals"][1]["id"],
            "title": "追击升级",
            "expected_revision": 3,
            "idempotency_key": "rename-valid",
        },
    )
    assert renamed.status_code == 200
    assert renamed.json()["data"]["plan"]["revision"] == 4
    assert renamed.json()["data"]["proposals"][1]["title"] == "追击升级"

    merged = await client.post(
        f"/api/v1/episode-plans/{result['plan']['id']}/merge",
        headers=headers,
        json={
            "left_proposal_id": renamed.json()["data"]["proposals"][1]["id"],
            "expected_revision": 4,
            "idempotency_key": "merge-valid",
        },
    )
    assert merged.status_code == 200
    merged_result = merged.json()["data"]
    assert merged_result["plan"]["revision"] == 5
    assert len(merged_result["proposals"]) == 2
    assert (
        "".join(
            text[item["source_start"] : item["source_end"]] for item in merged_result["proposals"]
        )
        == text
    )

    stale = await client.post(
        f"/api/v1/episode-plans/{result['plan']['id']}/confirm",
        headers=headers,
        json={"expected_revision": 4, "idempotency_key": "stale-confirm"},
    )
    assert stale.status_code == 409
    assert stale.json()["error"]["code"] == "version_conflict"

    async with session_factory() as session:
        assert await session.scalar(select(func.count()).select_from(Episode)) == 0
        assert await session.scalar(select(func.count()).select_from(ScriptSource)) == 0


@pytest.mark.asyncio
async def test_confirm_materialize_and_publish_are_concurrent_idempotent_batches(
    client: httpx.AsyncClient,
    session_factory: async_sessionmaker[AsyncSession],
) -> None:
    headers, project, document = await _project_and_document(
        client,
        email="episode-plan-materialize@example.com",
    )
    created = await _explicit_plan(
        client,
        headers,
        document["revision"]["id"],
        idempotency_key="materialize-plan",
    )
    plan = created.json()["data"]
    confirm_body = {"expected_revision": 1, "idempotency_key": "confirm-plan"}
    first_confirm, second_confirm = await asyncio.gather(
        client.post(
            f"/api/v1/episode-plans/{plan['plan']['id']}/confirm",
            headers=headers,
            json=confirm_body,
        ),
        client.post(
            f"/api/v1/episode-plans/{plan['plan']['id']}/confirm",
            headers=headers,
            json=confirm_body,
        ),
    )
    assert first_confirm.status_code == 200
    assert second_confirm.status_code == 200
    assert first_confirm.json()["data"] == second_confirm.json()["data"]
    confirmed = first_confirm.json()["data"]
    assert confirmed["plan"]["status"] == "confirmed"
    assert confirmed["plan"]["revision"] == 2

    materialize_body = {
        "mode": "append_new",
        "expected_plan_revision": 2,
        "expected_project_revision": confirmed["impact"]["project_revision"],
        "expected_active_order_hash": confirmed["impact"]["active_order_hash"],
        "idempotency_key": "materialize-batch",
    }
    materialized = await client.post(
        f"/api/v1/episode-plans/{plan['plan']['id']}/materializations",
        headers=headers,
        json=materialize_body,
    )
    assert materialized.status_code == 201
    batch = materialized.json()["data"]
    assert batch["commit"]["status"] == "materialized"
    assert batch["commit"]["revision"] == 2
    assert len(batch["segments"]) == 5
    assert all(item["published_version_id"] is None for item in batch["segments"])
    repeated_materialization = await client.post(
        f"/api/v1/episode-plans/{plan['plan']['id']}/materializations",
        headers=headers,
        json=materialize_body,
    )
    assert repeated_materialization.status_code == 201
    assert repeated_materialization.json()["data"] == batch

    episodes_after_draft = await client.get(
        f"/api/v1/projects/{project['id']}/episodes", headers=headers
    )
    assert episodes_after_draft.status_code == 200
    assert len(episodes_after_draft.json()["data"]) == 5
    assert all(
        episode["current_script_version_id"] is None
        for episode in episodes_after_draft.json()["data"]
    )

    publish_body = {
        "expected_revision": 2,
        "idempotency_key": "publish-batch",
    }
    async with session_factory() as session:
        commit_before_gate = await session.get(
            ImportCommit,
            UUID(batch["commit"]["id"]),
        )
        assert commit_before_gate is not None
        snapshot_before_gate = deepcopy(commit_before_gate.result_snapshot)

    blocked = await client.post(
        f"/api/v1/import-commits/{batch['commit']['id']}/publish",
        headers=headers,
        json=publish_body,
    )
    assert blocked.status_code == 409
    blocked_error = blocked.json()["error"]
    assert blocked_error["code"] == "state_conflict"
    assert blocked_error["message"] == (
        "Production Bible must be confirmed before episode publishing"
    )
    assert blocked_error["details"] == {"document_revision_id": document["revision"]["id"]}
    assert blocked_error["next_action"] == "confirm_production_bible"

    async with session_factory() as session:
        commit_after_gate = await session.get(
            ImportCommit,
            UUID(batch["commit"]["id"]),
        )
        assert commit_after_gate is not None
        assert commit_after_gate.status == "materialized"
        assert commit_after_gate.revision == 2
        assert commit_after_gate.publish_idempotency_key is None
        assert commit_after_gate.publish_input_hash is None
        assert commit_after_gate.result_snapshot == snapshot_before_gate
        assert await session.scalar(select(func.count()).select_from(ScriptVersion)) == 5

    bible_id = await seed_confirmed_production_bible(
        session_factory,
        workspace_id=UUID(batch["commit"]["workspace_id"]),
        project_id=UUID(project["id"]),
        document_revision_id=UUID(document["revision"]["id"]),
        input_hash=document["revision"]["normalized_hash"],
        actor_id=UUID(batch["commit"]["created_by"]),
    )
    published = await client.post(
        f"/api/v1/import-commits/{batch['commit']['id']}/publish",
        headers=headers,
        json=publish_body,
    )
    assert published.status_code == 200
    published_batch = published.json()["data"]
    assert published_batch["commit"]["status"] == "published"
    assert published_batch["commit"]["revision"] == 3
    assert all(item["published_version_id"] is not None for item in published_batch["segments"])
    repeated_publish = await client.post(
        f"/api/v1/import-commits/{batch['commit']['id']}/publish",
        headers=headers,
        json=publish_body,
    )
    assert repeated_publish.status_code == 200
    assert repeated_publish.json()["data"] == published_batch

    episodes_after_publish = await client.get(
        f"/api/v1/projects/{project['id']}/episodes", headers=headers
    )
    current_ids = {
        episode["id"]: episode["current_script_version_id"]
        for episode in episodes_after_publish.json()["data"]
    }
    assert all(current_ids.values())
    assert {
        item["episode_id"]: item["published_version_id"] for item in published_batch["segments"]
    } == current_ids

    source = document["revision"]["normalized_text"]
    async with session_factory() as session:
        assert await session.scalar(select(func.count()).select_from(Episode)) == 5
        assert await session.scalar(select(func.count()).select_from(ScriptSource)) == 5
        assert await session.scalar(select(func.count()).select_from(ScriptVersion)) == 10
        assert await session.scalar(select(func.count()).select_from(ExtractionBatch)) == 5
        assert (
            await session.scalar(
                select(func.count()).select_from(Task).where(Task.task_type == "script_extraction")
            )
            == 5
        )
        assert (
            await session.scalar(
                select(func.count())
                .select_from(OutboxEvent)
                .where(OutboxEvent.event_type == "script_extraction.requested")
            )
            == 5
        )
        assert await session.scalar(select(func.count()).select_from(NarrativeStructure)) == 5
        assert (
            await session.scalar(select(func.count()).select_from(NarrativeImpactAssessment)) == 5
        )
        assert await session.scalar(select(func.count()).select_from(ImportCommit)) == 1
        stored_commit = await session.get(ImportCommit, UUID(batch["commit"]["id"]))
        seeded_bible = await session.get(ProductionBible, bible_id)
        assert stored_commit is not None
        assert seeded_bible is not None
        assert stored_commit.result_snapshot["production_bible"] == {
            "id": str(seeded_bible.id),
            "document_revision_id": str(seeded_bible.document_revision_id),
            "revision": seeded_bible.revision,
            "result_hash": seeded_bible.result_hash,
        }
        extraction_batches = list(
            await session.scalars(select(ExtractionBatch).order_by(ExtractionBatch.id))
        )
        assert all(
            item.production_bible_id == seeded_bible.id
            and item.production_bible_revision == seeded_bible.revision
            and item.production_bible_result_hash == seeded_bible.result_hash
            and item.script_content_hash != item.input_hash
            for item in extraction_batches
        )
        origins = list(
            await session.scalars(
                select(EpisodeSegmentOrigin).order_by(EpisodeSegmentOrigin.position)
            )
        )
        assert len(origins) == 5
        assert "".join(source[item.source_start : item.source_end] for item in origins) == source
        for origin in origins:
            draft = await session.get(ScriptVersion, origin.draft_version_id)
            published_version = await session.get(ScriptVersion, origin.published_version_id)
            assert draft is not None and draft.status == "draft"
            assert published_version is not None and published_version.status == "published"
            assert draft.body == source[origin.source_start : origin.source_end]
            assert published_version.body == draft.body
            assert published_version.content_hash == draft.content_hash


@pytest.mark.asyncio
async def test_materialization_failure_on_third_segment_rolls_back_every_formal_write(
    client: httpx.AsyncClient,
    session_factory: async_sessionmaker[AsyncSession],
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    from app.modules.scripts.planning import service as planning_service

    headers, _, document = await _project_and_document(
        client,
        email="episode-plan-rollback@example.com",
    )
    created = await _explicit_plan(
        client,
        headers,
        document["revision"]["id"],
        idempotency_key="rollback-plan",
    )
    plan = created.json()["data"]
    confirmed_response = await client.post(
        f"/api/v1/episode-plans/{plan['plan']['id']}/confirm",
        headers=headers,
        json={"expected_revision": 1, "idempotency_key": "rollback-confirm"},
    )
    confirmed = confirmed_response.json()["data"]
    real_write = planning_service.__dict__["_write_materialized_segment"]
    calls = 0

    async def fail_third(*args: Any, **kwargs: Any) -> Any:
        nonlocal calls
        calls += 1
        if calls == 3:
            raise RuntimeError("synthetic third segment failure")
        return await real_write(*args, **kwargs)

    monkeypatch.setattr(planning_service, "_write_materialized_segment", fail_third)
    failed = await client.post(
        f"/api/v1/episode-plans/{plan['plan']['id']}/materializations",
        headers=headers,
        json={
            "mode": "append_new",
            "expected_plan_revision": 2,
            "expected_project_revision": confirmed["impact"]["project_revision"],
            "expected_active_order_hash": confirmed["impact"]["active_order_hash"],
            "idempotency_key": "rollback-materialize",
        },
    )
    assert failed.status_code == 500
    assert failed.json()["error"]["code"] == "internal_error"
    async with session_factory() as session:
        assert await session.scalar(select(func.count()).select_from(Episode)) == 0
        assert await session.scalar(select(func.count()).select_from(ScriptSource)) == 0
        assert await session.scalar(select(func.count()).select_from(ScriptVersion)) == 0
        assert await session.scalar(select(func.count()).select_from(EpisodeSegmentOrigin)) == 0
        commit = await session.scalar(select(ImportCommit))
        assert commit is not None
        assert commit.status == "failed"
        assert commit.error_code == "materialization_failed"


class _RecordingMessage:
    def __init__(self, body: bytes) -> None:
        self.body = body
        self.ack_count = 0
        self.nack_requeues: list[bool] = []

    async def ack(self) -> None:
        self.ack_count += 1

    async def nack(self, *, requeue: bool) -> None:
        self.nack_requeues.append(requeue)


class _RecordingEpisodePlanner:
    def __init__(self) -> None:
        self.inputs: list[str] = []

    async def plan(
        self,
        normalized_text: str,
        *,
        target_duration_ms: int,
        maximum_episode_count: int,
    ) -> EpisodePlanningProviderResult:
        self.inputs.append(normalized_text)
        lines = normalized_text.splitlines(keepends=True)
        return EpisodePlanningProviderResult(
            proposals=[
                EpisodePlanningProviderProposal(
                    title="警报出现",
                    end_block_position=3,
                    exact_end_anchor=lines[2].rstrip("\r\n")[-24:],
                    estimated_duration_ms=min(target_duration_ms, 60_000),
                    reason="冲突建立并以新线索收束",
                    confidence=0.86,
                ),
                EpisodePlanningProviderProposal(
                    title="追踪真相",
                    end_block_position=len(lines),
                    exact_end_anchor=lines[-1][-24:],
                    estimated_duration_ms=min(target_duration_ms, 60_000),
                    reason="完成追踪并留下下一集钩子",
                    confidence=0.82,
                ),
            ]
        )


class _InvalidAnchorEpisodePlanner:
    async def plan(
        self,
        normalized_text: str,
        *,
        target_duration_ms: int,
        maximum_episode_count: int,
    ) -> EpisodePlanningProviderResult:
        return EpisodePlanningProviderResult(
            proposals=[
                EpisodePlanningProviderProposal(
                    title="非法候选",
                    end_block_position=len(normalized_text.splitlines()),
                    exact_end_anchor="原文中不存在的锚点",
                    estimated_duration_ms=target_duration_ms,
                    reason="故意构造非法锚点",
                    confidence=0.5,
                )
            ]
        )


async def _queued_ai_plan(
    client: httpx.AsyncClient,
    session_factory: async_sessionmaker[AsyncSession],
    *,
    email: str,
    idempotency_key: str,
) -> tuple[dict[str, str], dict[str, Any], bytes]:
    text = (
        "场景1：控制室，夜\n警报突然响起。\n甲：先封锁三号门。\n"
        "场景2：港口，雨\n乙发现被删掉的日志。\n远处灯塔熄灭。"
    )
    headers, _, document = await _project_and_document(client, email=email, text=text)
    created = await client.post(
        f"/api/v1/document-revisions/{document['revision']['id']}/episode-plans",
        headers=headers,
        json={
            "strategy": "target_duration_ai",
            "target_duration_ms": 60_000,
            "requested_episode_count": None,
            "idempotency_key": idempotency_key,
        },
    )
    assert created.status_code == 202
    queued = created.json()["data"]
    async with session_factory() as session:
        event = await session.scalar(
            select(OutboxEvent).where(
                OutboxEvent.aggregate_id == UUID(queued["plan"]["planning_task_id"])
            )
        )
        assert event is not None
        body = envelope_from_event(event).model_dump_json().encode()
    return headers, queued, body


@pytest.mark.asyncio
async def test_unmarked_document_ai_candidate_is_async_validated_and_never_writes_episode(
    client: httpx.AsyncClient,
    session_factory: async_sessionmaker[AsyncSession],
) -> None:
    text = (
        "场景1：控制室，夜\n警报突然响起。\n甲：先封锁三号门。\n"
        "场景2：港口，雨\n乙发现被删掉的日志。\n远处灯塔熄灭。"
    )
    headers, _, document = await _project_and_document(
        client,
        email="episode-plan-ai@example.com",
        text=text,
    )
    created = await client.post(
        f"/api/v1/document-revisions/{document['revision']['id']}/episode-plans",
        headers=headers,
        json={
            "strategy": "target_duration_ai",
            "target_duration_ms": 60_000,
            "requested_episode_count": None,
            "idempotency_key": "ai-plan",
        },
    )
    assert created.status_code == 202
    queued = created.json()["data"]
    assert queued["plan"]["status"] == "draft"
    assert queued["plan"]["planning_task_id"] is not None
    assert queued["proposals"] == []
    async with session_factory() as session:
        event = await session.scalar(
            select(OutboxEvent).where(
                OutboxEvent.aggregate_id == UUID(queued["plan"]["planning_task_id"])
            )
        )
        assert event is not None
        assert event.event_type == "episode_planning.requested"
        assert event.payload == {"task_id": queued["plan"]["planning_task_id"]}
        assert "控制室" not in str(event.payload)
        body = envelope_from_event(event).model_dump_json().encode()

    planner = _RecordingEpisodePlanner()
    message = _RecordingMessage(body)
    outcome = await io_worker.process_incoming_message(
        message,
        session_factory,
        episode_planner=planner,
    )
    assert outcome == "completed"
    assert message.ack_count == 1
    assert message.nack_requeues == []
    assert planner.inputs == [text]

    fetched = await client.get(f"/api/v1/episode-plans/{queued['plan']['id']}", headers=headers)
    assert fetched.status_code == 200
    candidate = fetched.json()["data"]
    assert candidate["plan"]["status"] == "review_ready"
    assert candidate["plan"]["revision"] == 2
    assert len(candidate["proposals"]) == 2
    assert all(item["boundary_evidence"]["kind"] == "ai_anchor" for item in candidate["proposals"])
    assert (
        "".join(text[item["source_start"] : item["source_end"]] for item in candidate["proposals"])
        == text
    )
    async with session_factory() as session:
        assert await session.scalar(select(func.count()).select_from(Episode)) == 0


@pytest.mark.asyncio
async def test_ai_plan_without_configured_provider_fails_once_without_formal_writes(
    client: httpx.AsyncClient,
    session_factory: async_sessionmaker[AsyncSession],
) -> None:
    headers, queued, body = await _queued_ai_plan(
        client,
        session_factory,
        email="episode-plan-no-provider@example.com",
        idempotency_key="ai-plan-no-provider",
    )
    message = _RecordingMessage(body)

    outcome = await io_worker.process_incoming_message(
        message,
        session_factory,
        episode_planner=None,
    )

    assert outcome == "completed"
    assert message.ack_count == 1
    assert message.nack_requeues == []
    fetched = await client.get(
        f"/api/v1/episode-plans/{queued['plan']['id']}",
        headers=headers,
    )
    assert fetched.status_code == 200
    failed_plan = fetched.json()["data"]
    assert failed_plan["plan"]["status"] == "draft"
    assert failed_plan["plan"]["planning_error_code"] == "ai_service_unavailable"
    assert failed_plan["proposals"] == []

    async with session_factory() as session:
        task = await session.get(Task, UUID(queued["plan"]["planning_task_id"]))
        assert task is not None
        assert task.status == "failed"
        assert task.error_code == "ai_service_unavailable"
        delivery = await session.scalar(select(InboxDelivery))
        assert delivery is not None
        assert delivery.status == "completed"
        assert delivery.attempt_count == 1
        assert await session.scalar(select(func.count()).select_from(Episode)) == 0


@pytest.mark.asyncio
async def test_ai_plan_with_invalid_anchor_is_acked_as_non_retryable_failure(
    client: httpx.AsyncClient,
    session_factory: async_sessionmaker[AsyncSession],
) -> None:
    headers, queued, body = await _queued_ai_plan(
        client,
        session_factory,
        email="episode-plan-invalid-anchor@example.com",
        idempotency_key="ai-plan-invalid-anchor",
    )
    message = _RecordingMessage(body)

    outcome = await io_worker.process_incoming_message(
        message,
        session_factory,
        episode_planner=_InvalidAnchorEpisodePlanner(),
    )

    assert outcome == "completed"
    assert message.ack_count == 1
    assert message.nack_requeues == []
    fetched = await client.get(
        f"/api/v1/episode-plans/{queued['plan']['id']}",
        headers=headers,
    )
    assert fetched.status_code == 200
    failed_plan = fetched.json()["data"]
    assert failed_plan["plan"]["status"] == "draft"
    assert failed_plan["plan"]["planning_error_code"] == "ai_output_invalid"
    assert failed_plan["proposals"] == []

    async with session_factory() as session:
        task = await session.get(Task, UUID(queued["plan"]["planning_task_id"]))
        assert task is not None
        assert task.status == "failed"
        assert task.error_code == "ai_output_invalid"
        delivery = await session.scalar(select(InboxDelivery))
        assert delivery is not None
        assert delivery.status == "completed"
        assert delivery.last_error == "ai_output_invalid"
        assert delivery.attempt_count == 1
        assert await session.scalar(select(func.count()).select_from(Episode)) == 0


@pytest.mark.asyncio
async def test_ai_plan_redelivery_after_preparation_is_marked_unknown_not_resubmitted(
    client: httpx.AsyncClient,
    session_factory: async_sessionmaker[AsyncSession],
) -> None:
    headers, queued, body = await _queued_ai_plan(
        client,
        session_factory,
        email="episode-plan-interrupted@example.com",
        idempotency_key="ai-plan-interrupted",
    )
    envelope = MessageEnvelope.model_validate_json(body)
    async with session_factory() as session:
        async with session.begin():
            prepared = await prepare_episode_planning(
                session,
                envelope,
                configured=True,
            )
            assert isinstance(prepared, PreparedEpisodePlanning)

    planner = _RecordingEpisodePlanner()
    message = _RecordingMessage(body)
    outcome = await io_worker.process_incoming_message(
        message,
        session_factory,
        episode_planner=planner,
    )

    assert outcome == "completed"
    assert message.ack_count == 1
    assert message.nack_requeues == []
    assert planner.inputs == []
    fetched = await client.get(
        f"/api/v1/episode-plans/{queued['plan']['id']}",
        headers=headers,
    )
    assert fetched.status_code == 200
    interrupted_plan = fetched.json()["data"]
    assert interrupted_plan["plan"]["status"] == "draft"
    assert interrupted_plan["plan"]["planning_error_code"] == "ai_result_unknown"
    assert interrupted_plan["proposals"] == []

    async with session_factory() as session:
        task = await session.get(Task, UUID(queued["plan"]["planning_task_id"]))
        assert task is not None
        assert task.status == "unknown"
        assert task.error_code == "ai_result_unknown"
        delivery = await session.scalar(select(InboxDelivery))
        assert delivery is not None
        assert delivery.status == "completed"
        assert delivery.last_error == "ai_result_unknown"
        assert delivery.attempt_count == 2
        assert await session.scalar(select(func.count()).select_from(Episode)) == 0
