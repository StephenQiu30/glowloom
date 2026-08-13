from copy import deepcopy
from typing import Any
from uuid import UUID

import httpx
import pytest
from sqlalchemy import func, select
from sqlalchemy.ext.asyncio import AsyncSession, async_sessionmaker
from uuid6 import uuid7

from app.core.errors import ApiError
from app.modules.scripts.models import Dialogue, Scene
from app.modules.scripts.narratives.models import NarrativeUnit, NarrativeUnitVersion
from app.modules.storyboards.drafts import record_draft_result
from app.modules.storyboards.drafts.models import DraftShot
from app.modules.storyboards.drafts.schemas import DraftProviderResult
from app.modules.storyboards.models import Shot
from tests.support.identity_builders import register_identity_response
from tests.support.project_builders import project_payload


async def published_episode(
    client: httpx.AsyncClient,
    session_factory: async_sessionmaker[AsyncSession],
    *,
    email: str,
) -> tuple[dict[str, str], dict[str, Any], dict[str, Any], dict[str, Any]]:
    registered = await register_identity_response(client, email=email)
    assert registered.status_code == 201
    identity = registered.json()["data"]
    headers = {"authorization": f"Bearer {identity['access_token']}"}

    created_project = await client.post(
        "/api/v1/projects",
        headers=headers,
        json=project_payload(identity["workspace"]["id"], "AI 分镜草案验收项目"),
    )
    assert created_project.status_code == 201
    project = created_project.json()["data"]
    created_episode = await client.post(
        f"/api/v1/projects/{project['id']}/episodes",
        headers=headers,
        json={"name": "第一集", "target_duration_ms": 11_000},
    )
    assert created_episode.status_code == 201
    episode = created_episode.json()["data"]

    body = (
        "第一集\n"
        "内景·旧车站·夜\n"
        "林澈沿着积水的月台缓慢前行。\n"
        "林澈：有人吗？\n"
        "广播突然响起，警示她不要回头。"
    )
    imported = await client.post(
        f"/api/v1/episodes/{episode['id']}/script-sources",
        headers=headers,
        json={
            "input_type": "text",
            "title": "分镜草案验收稿",
            "body": body,
            "rights_declaration": "原创合成测试文本",
            "idempotency_key": f"draft-source:{email}",
        },
    )
    assert imported.status_code == 201
    source = imported.json()["data"]["source"]
    published = await client.post(
        f"/api/v1/script-sources/{source['id']}/versions",
        headers=headers,
        json={"body": body, "expected_current_version_id": None},
    )
    assert published.status_code == 201
    version = published.json()["data"]["version"]
    async with session_factory() as session, session.begin():
        scene = Scene(
            workspace_id=UUID(identity["workspace"]["id"]),
            script_version_id=UUID(version["id"]),
            position=1,
            heading="内景·旧车站·夜",
            location="旧车站月台",
            time_of_day="夜",
            summary="林澈在雨夜进入旧车站",
            source_start=0,
            source_end=len(body),
        )
        session.add(scene)
        await session.flush()
        dialogue_start = body.index("林澈：有人吗？")
        dialogue = Dialogue(
            workspace_id=UUID(identity["workspace"]["id"]),
            scene_id=scene.id,
            position=1,
            speaker_candidate="林澈",
            dialogue_kind="spoken",
            text="有人吗？",
            source_start=dialogue_start,
            source_end=dialogue_start + len("林澈：有人吗？"),
        )
        session.add(dialogue)
        await session.flush()
        rows = await session.execute(
            select(NarrativeUnitVersion, NarrativeUnit)
            .join(NarrativeUnit, NarrativeUnit.id == NarrativeUnitVersion.unit_id)
            .where(NarrativeUnitVersion.script_version_id == UUID(version["id"]))
        )
        for unit_version, unit in rows:
            unit_version.source_scene_id = scene.id
            if unit.kind == "dialogue":
                unit_version.source_dialogue_id = dialogue.id
    structure_response = await client.get(
        f"/api/v1/script-versions/{version['id']}/narrative-structure",
        headers=headers,
    )
    assert structure_response.status_code == 200
    structure = structure_response.json()["data"]
    return (
        headers,
        project,
        episode,
        {
            "version": version,
            "structure": structure,
            "actor_id": identity["user"]["id"],
            "workspace_id": identity["workspace"]["id"],
        },
    )


def draft_spec(
    structure: dict[str, Any],
    *,
    purpose: str,
    duration_ms: int,
) -> dict[str, object]:
    scene_id = next(
        unit["source_scene_id"]
        for unit in structure["units"]
        if unit["source_scene_id"] is not None
    )
    dialogue_ids = [
        unit["source_dialogue_id"]
        for unit in structure["units"]
        if unit["source_dialogue_id"] is not None
    ]
    return {
        "schema_version": 1,
        "script_reference": {
            "confirmed_script_version_id": structure["script_version_id"],
            "scene_id": scene_id,
            "dialogue_ids": dialogue_ids,
        },
        "narrative": {"purpose": purpose, "continuity_note": None},
        "visual": {
            "shot_size": "medium",
            "camera_angle": "eye_level",
            "camera_movement": "dolly",
            "composition": "林澈位于画面中央并朝纵深移动",
            "environment": "雨夜废弃车站月台",
            "subject_placements": [{"subject_key": "hero", "placement": "画面中央"}],
            "mood_lighting": "冷蓝顶光",
        },
        "action_beats": [{"beat_key": "walk", "order": 1, "description": "林澈缓慢前行"}],
        "dialogue_or_narration": [
            {
                "source_dialogue_id": dialogue_id,
                "beat_key": "walk",
                "speaker_subject_key": "hero",
                "render_as_audio": True,
                "performance_note": "压低声音",
            }
            for dialogue_id in dialogue_ids
        ],
        "duration_ms": duration_ms,
        "audio_intent": {"ambient": "雨声", "sound_effects": []},
        "generation_intent": {
            "mode": "text_to_video",
            "first_frame": None,
            "last_frame": None,
            "keyframe_notes": None,
        },
    }


def provider_result(structure: dict[str, Any]) -> DraftProviderResult:
    unit_ids = [unit["id"] for unit in structure["units"]]
    return DraftProviderResult.model_validate(
        {
            "shots": [
                {
                    "proposal_key": "opening-walk",
                    "position": 1,
                    "title": "雨夜进入月台",
                    "narrative_unit_version_ids": unit_ids[:-1],
                    "spec": draft_spec(
                        structure,
                        purpose="建立雨夜车站与人物处境",
                        duration_ms=6_000,
                    ),
                    "asset_references": [],
                    "risk_codes": [],
                },
                {
                    "proposal_key": "warning-broadcast",
                    "position": 2,
                    "title": "广播发出警告",
                    "narrative_unit_version_ids": unit_ids[-1:],
                    "spec": draft_spec(
                        structure,
                        purpose="用广播制造悬念",
                        duration_ms=5_000,
                    ),
                    "asset_references": [],
                    "risk_codes": ["audio_continuity_review"],
                },
            ]
        }
    )


async def create_batch_fixture(
    client: httpx.AsyncClient,
    *,
    headers: dict[str, str],
    episode: dict[str, Any],
    version_id: str,
    key: str,
) -> dict[str, Any]:
    response = await client.post(
        f"/api/v1/episodes/{episode['id']}/storyboard-draft-batches",
        headers=headers,
        json={
            "input_script_version_id": version_id,
            "asset_state_ids": [],
            "idempotency_key": key,
        },
    )
    assert response.status_code == 202
    return response.json()["data"]


async def record_result_fixture(
    session_factory: async_sessionmaker[AsyncSession],
    *,
    batch_id: str,
    structure: dict[str, Any],
) -> None:
    async with session_factory() as session, session.begin():
        await record_draft_result(
            session,
            batch_id=UUID(batch_id),
            result=provider_result(structure),
        )


@pytest.mark.asyncio
async def test_provider_result_requires_complete_narrative_coverage(
    client: httpx.AsyncClient,
    session_factory: async_sessionmaker[AsyncSession],
) -> None:
    headers, _project, episode, script = await published_episode(
        client,
        session_factory,
        email="draft-coverage@example.com",
    )
    batch = await create_batch_fixture(
        client,
        headers=headers,
        episode=episode,
        version_id=script["version"]["id"],
        key="draft-coverage",
    )
    payload = provider_result(script["structure"]).model_dump(mode="json")
    payload["shots"] = payload["shots"][:1]
    payload["shots"][0]["spec"]["duration_ms"] = 11_000

    with pytest.raises(ApiError, match="cover every required narrative unit"):
        async with session_factory() as session, session.begin():
            await record_draft_result(
                session,
                batch_id=UUID(batch["id"]),
                result=payload,
            )

    async with session_factory() as session:
        draft_count = await session.scalar(select(func.count()).select_from(DraftShot))
        shot_count = await session.scalar(select(func.count()).select_from(Shot))
        assert draft_count == 0
        assert shot_count == 0


@pytest.mark.asyncio
async def test_drafts_require_complete_review_before_atomic_apply(
    client: httpx.AsyncClient,
    session_factory: async_sessionmaker[AsyncSession],
) -> None:
    headers, _project, episode, script = await published_episode(
        client,
        session_factory,
        email="draft-apply@example.com",
    )
    batch = await create_batch_fixture(
        client,
        headers=headers,
        episode=episode,
        version_id=script["version"]["id"],
        key="draft-batch:apply:1",
    )
    assert batch["status"] == "queued"
    assert batch["drafts"] == []

    before_result = await client.get(
        f"/api/v1/episodes/{episode['id']}/shots",
        headers=headers,
    )
    assert before_result.status_code == 200
    assert before_result.json()["data"]["items"] == []

    await record_result_fixture(
        session_factory,
        batch_id=batch["id"],
        structure=script["structure"],
    )
    fetched = await client.get(
        f"/api/v1/storyboard-draft-batches/{batch['id']}",
        headers=headers,
    )
    assert fetched.status_code == 200
    batch = fetched.json()["data"]
    assert batch["status"] == "needs_review"
    assert len(batch["drafts"]) == 2
    assert batch["decision_summary"] == {
        "pending": 2,
        "accepted": 0,
        "modified": 0,
        "ignored": 0,
    }

    after_result = await client.get(
        f"/api/v1/episodes/{episode['id']}/shots",
        headers=headers,
    )
    assert after_result.status_code == 200
    assert after_result.json()["data"]["items"] == []

    first, second = batch["drafts"]
    first_decision = await client.post(
        f"/api/v1/storyboard-drafts/{first['id']}/decisions",
        headers=headers,
        json={
            "action": "accepted",
            "expected_batch_revision": batch["revision"],
            "idempotency_key": "draft-decision:first:accept",
        },
    )
    assert first_decision.status_code == 201
    batch = first_decision.json()["data"]["batch"]

    incomplete = await client.post(
        f"/api/v1/storyboard-draft-batches/{batch['id']}/approve",
        headers=headers,
        json={
            "expected_revision": batch["revision"],
            "idempotency_key": "draft-approve:incomplete",
        },
    )
    assert incomplete.status_code == 409
    assert incomplete.json()["error"]["code"] == "state_conflict"

    second_decision = await client.post(
        f"/api/v1/storyboard-drafts/{second['id']}/decisions",
        headers=headers,
        json={
            "action": "ignored",
            "expected_batch_revision": batch["revision"],
            "idempotency_key": "draft-decision:second:ignore",
        },
    )
    assert second_decision.status_code == 201
    batch = second_decision.json()["data"]["batch"]
    approved_response = await client.post(
        f"/api/v1/storyboard-draft-batches/{batch['id']}/approve",
        headers=headers,
        json={
            "expected_revision": batch["revision"],
            "idempotency_key": "draft-approve:complete",
        },
    )
    assert approved_response.status_code == 200
    approved = approved_response.json()["data"]
    assert approved["status"] == "approved"

    preflight_response = await client.post(
        f"/api/v1/storyboard-draft-batches/{batch['id']}/apply-preflight",
        headers=headers,
        json={"expected_revision": approved["revision"]},
    )
    assert preflight_response.status_code == 200
    preflight = preflight_response.json()["data"]
    assert preflight["diff"] == {
        "kept": 0,
        "created": 1,
        "modified": 0,
        "archived": 0,
    }

    command = {
        "expected_revision": approved["revision"],
        "expected_order_hash": preflight["order_hash"],
        "impact_hash": preflight["impact_hash"],
        "idempotency_key": "draft-apply:complete",
    }
    applied_response = await client.post(
        f"/api/v1/storyboard-draft-batches/{batch['id']}/apply",
        headers=headers,
        json=command,
    )
    assert applied_response.status_code == 201
    applied = applied_response.json()["data"]
    assert applied["batch"]["status"] == "applied"
    assert len(applied["created_shot_ids"]) == 1

    replay = await client.post(
        f"/api/v1/storyboard-draft-batches/{batch['id']}/apply",
        headers=headers,
        json=command,
    )
    assert replay.status_code == 201
    assert replay.json()["data"] == applied

    shots = await client.get(
        f"/api/v1/episodes/{episode['id']}/shots",
        headers=headers,
    )
    assert shots.status_code == 200
    items = shots.json()["data"]["items"]
    assert len(items) == 1
    assert items[0]["source_draft_shot_id"] == first["id"]


@pytest.mark.asyncio
async def test_apply_rejects_shot_baseline_change_without_partial_writes(
    client: httpx.AsyncClient,
    session_factory: async_sessionmaker[AsyncSession],
) -> None:
    headers, _project, episode, script = await published_episode(
        client,
        session_factory,
        email="draft-conflict@example.com",
    )
    batch = await create_batch_fixture(
        client,
        headers=headers,
        episode=episode,
        version_id=script["version"]["id"],
        key="draft-batch:conflict:1",
    )
    await record_result_fixture(
        session_factory,
        batch_id=batch["id"],
        structure=script["structure"],
    )
    fetched = await client.get(
        f"/api/v1/storyboard-draft-batches/{batch['id']}",
        headers=headers,
    )
    batch = fetched.json()["data"]
    for index, draft in enumerate(batch["drafts"]):
        decision = await client.post(
            f"/api/v1/storyboard-drafts/{draft['id']}/decisions",
            headers=headers,
            json={
                "action": "accepted",
                "expected_batch_revision": batch["revision"],
                "idempotency_key": f"draft-conflict:accept:{index}",
            },
        )
        assert decision.status_code == 201
        batch = decision.json()["data"]["batch"]
    approved_response = await client.post(
        f"/api/v1/storyboard-draft-batches/{batch['id']}/approve",
        headers=headers,
        json={
            "expected_revision": batch["revision"],
            "idempotency_key": "draft-conflict:approve",
        },
    )
    assert approved_response.status_code == 200
    approved = approved_response.json()["data"]
    preflight_response = await client.post(
        f"/api/v1/storyboard-draft-batches/{batch['id']}/apply-preflight",
        headers=headers,
        json={"expected_revision": approved["revision"]},
    )
    assert preflight_response.status_code == 200
    preflight = preflight_response.json()["data"]

    structure = script["structure"]
    scene_id = next(
        unit["source_scene_id"]
        for unit in structure["units"]
        if unit["source_scene_id"] is not None
    )
    async with session_factory() as session, session.begin():
        session.add(
            Shot(
                id=uuid7(),
                workspace_id=UUID(script["workspace_id"]),
                episode_id=UUID(episode["id"]),
                position=1,
                title="审核期间新增的人工镜头",
                source_script_version_id=UUID(script["version"]["id"]),
                source_scene_id=UUID(scene_id),
                source_candidate_id=None,
                source_draft_shot_id=None,
                creation_key="manual-after-draft-preflight",
                status="active",
                revision=1,
                created_by=UUID(script["actor_id"]),
            )
        )

    rejected = await client.post(
        f"/api/v1/storyboard-draft-batches/{batch['id']}/apply",
        headers=headers,
        json={
            "expected_revision": approved["revision"],
            "expected_order_hash": preflight["order_hash"],
            "impact_hash": preflight["impact_hash"],
            "idempotency_key": "draft-conflict:apply",
        },
    )
    assert rejected.status_code == 409
    assert rejected.json()["error"]["code"] == "version_conflict"

    shots = await client.get(
        f"/api/v1/episodes/{episode['id']}/shots",
        headers=headers,
    )
    assert shots.status_code == 200
    assert [item["title"] for item in shots.json()["data"]["items"]] == ["审核期间新增的人工镜头"]


@pytest.mark.asyncio
async def test_decisions_are_append_only_and_idempotency_is_scoped_to_input(
    client: httpx.AsyncClient,
    session_factory: async_sessionmaker[AsyncSession],
) -> None:
    headers, _project, episode, script = await published_episode(
        client,
        session_factory,
        email="draft-decisions@example.com",
    )
    batch = await create_batch_fixture(
        client,
        headers=headers,
        episode=episode,
        version_id=script["version"]["id"],
        key="draft-batch:decisions:1",
    )
    await record_result_fixture(
        session_factory,
        batch_id=batch["id"],
        structure=script["structure"],
    )
    fetched = await client.get(
        f"/api/v1/storyboard-draft-batches/{batch['id']}",
        headers=headers,
    )
    batch = fetched.json()["data"]
    draft = batch["drafts"][0]
    accepted_command = {
        "action": "accepted",
        "expected_batch_revision": batch["revision"],
        "idempotency_key": "draft-decision:append-only",
    }
    accepted = await client.post(
        f"/api/v1/storyboard-drafts/{draft['id']}/decisions",
        headers=headers,
        json=accepted_command,
    )
    assert accepted.status_code == 201
    accepted_data = accepted.json()["data"]

    replay = await client.post(
        f"/api/v1/storyboard-drafts/{draft['id']}/decisions",
        headers=headers,
        json=accepted_command,
    )
    assert replay.status_code == 201
    assert replay.json()["data"] == accepted_data

    conflicting = deepcopy(accepted_command)
    conflicting["action"] = "ignored"
    rejected = await client.post(
        f"/api/v1/storyboard-drafts/{draft['id']}/decisions",
        headers=headers,
        json=conflicting,
    )
    assert rejected.status_code == 409
    assert rejected.json()["error"]["code"] == "resource_conflict"

    revised = await client.post(
        f"/api/v1/storyboard-drafts/{draft['id']}/decisions",
        headers=headers,
        json={
            "action": "ignored",
            "expected_batch_revision": accepted_data["batch"]["revision"],
            "idempotency_key": "draft-decision:append-only:2",
        },
    )
    assert revised.status_code == 201
    history = revised.json()["data"]["draft"]["decision_history"]
    assert [item["action"] for item in history] == ["accepted", "ignored"]
