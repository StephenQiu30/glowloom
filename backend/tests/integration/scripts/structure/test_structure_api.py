import asyncio
from typing import Any
from uuid import UUID

import httpx
import pytest
from sqlalchemy import func, select
from sqlalchemy.ext.asyncio import AsyncSession, async_sessionmaker

from app.core.database import Base
from app.modules.assets.models import Asset, AssetVersion
from app.modules.projects.models import Episode
from app.modules.scripts.extractions import schemas as extraction_schemas
from app.modules.scripts.extractions import service as extraction_service
from tests.support.identity_builders import register_identity_response
from tests.support.project_builders import project_payload

SCRIPT_BODY = "A" * 100


def _candidate(
    key: str,
    start: int,
    end: int,
    proposal: dict[str, Any],
) -> dict[str, Any]:
    return {
        "candidate_key": key,
        "source_range": {"start": start, "end": end},
        "proposal": proposal,
        "confidence_note": None,
    }


def _blocking_candidates() -> list[dict[str, Any]]:
    return [
        _candidate(
            "scene-001",
            0,
            10,
            {
                "kind": "scene",
                "heading": "第一场",
                "location": "客厅",
                "time_of_day": "白天",
                "summary": "角色开始行动",
            },
        ),
        _candidate(
            "dialogue-001",
            11,
            20,
            {
                "kind": "dialogue",
                "scene_candidate_key": "scene-001",
                "speaker_candidate": "角色甲",
                "dialogue_kind": "spoken",
                "text": "开始。",
                "performance_note": "坚定",
            },
        ),
        _candidate(
            "asset-001",
            21,
            30,
            {
                "kind": "asset",
                "asset_kind": "character",
                "name": "角色甲",
                "description": "行动发起者",
            },
        ),
        _candidate(
            "continuity-001",
            31,
            40,
            {
                "kind": "continuity",
                "severity": "blocking",
                "issue": "角色动机缺失",
                "suggestion": "确认行动原因",
            },
        ),
    ]


def _ordered_candidates() -> list[dict[str, Any]]:
    return [
        _candidate(
            "scene-late",
            51,
            60,
            {
                "kind": "scene",
                "heading": "第二场",
                "location": "街道",
                "time_of_day": "夜晚",
                "summary": "角色抵达街道",
            },
        ),
        _candidate(
            "scene-early",
            0,
            10,
            {
                "kind": "scene",
                "heading": "第一场",
                "location": "客厅",
                "time_of_day": "白天",
                "summary": "原始摘要",
            },
        ),
        _candidate(
            "scene-alias",
            11,
            20,
            {
                "kind": "scene",
                "heading": "第一场补充",
                "location": "客厅",
                "time_of_day": "白天",
                "summary": "应合并到第一场",
            },
        ),
        _candidate(
            "dialogue-alias",
            21,
            30,
            {
                "kind": "dialogue",
                "scene_candidate_key": "scene-alias",
                "speaker_candidate": "角色甲",
                "dialogue_kind": "spoken",
                "text": "开始。",
                "performance_note": "坚定",
            },
        ),
        _candidate(
            "dialogue-ignore",
            61,
            70,
            {
                "kind": "dialogue",
                "scene_candidate_key": "scene-late",
                "speaker_candidate": "角色乙",
                "dialogue_kind": "spoken",
                "text": "忽略这句。",
                "performance_note": None,
            },
        ),
        _candidate(
            "asset-pending",
            71,
            80,
            {
                "kind": "asset",
                "asset_kind": "character",
                "name": "角色甲",
                "description": "下游资产候选",
            },
        ),
    ]


async def _completed_batch(
    client: httpx.AsyncClient,
    session_factory: async_sessionmaker[AsyncSession],
    *,
    email: str,
    key: str,
    candidates: list[dict[str, Any]],
) -> tuple[dict[str, str], dict[str, Any], dict[str, Any], list[dict[str, Any]]]:
    registration = await register_identity_response(
        client,
        email=email,
        password="a-secure-structure-password",
        display_name="结构负责人",
    )
    assert registration.status_code == 201
    identity = registration.json()["data"]
    headers = {"authorization": f"Bearer {identity['access_token']}"}
    workspace_id = identity["workspace"]["id"]

    project_response = await client.post(
        "/api/v1/projects",
        headers=headers,
        json=project_payload(workspace_id, "结构确认项目"),
    )
    assert project_response.status_code == 201
    project = project_response.json()["data"]
    episode_response = await client.post(
        f"/api/v1/projects/{project['id']}/episodes",
        headers=headers,
        json={"name": "结构确认单集", "target_duration_ms": 90000},
    )
    assert episode_response.status_code == 201
    episode = episode_response.json()["data"]

    imported_response = await client.post(
        f"/api/v1/episodes/{episode['id']}/script-sources",
        headers=headers,
        json={
            "input_type": "text",
            "title": "待确认剧本",
            "body": SCRIPT_BODY,
            "rights_declaration": "确认拥有测试文本使用权",
            "idempotency_key": f"{key}-import",
        },
    )
    assert imported_response.status_code == 201
    source = imported_response.json()["data"]["source"]
    published_response = await client.post(
        f"/api/v1/script-sources/{source['id']}/versions",
        headers=headers,
        json={"body": SCRIPT_BODY, "expected_current_version_id": None},
    )
    assert published_response.status_code == 201
    input_version = published_response.json()["data"]["version"]

    extraction_response = await client.post(
        f"/api/v1/script-versions/{input_version['id']}/extractions",
        headers=headers,
        json={"scope": "full", "idempotency_key": f"{key}-extraction"},
    )
    assert extraction_response.status_code == 202
    batch = extraction_response.json()["data"]
    result = extraction_schemas.ScriptExtractionResult.model_validate(
        {"candidates": candidates}
    )
    async with session_factory() as session:
        async with session.begin():
            await extraction_service.record_extraction_result(
                session,
                UUID(batch["id"]),
                result,
            )
    candidate_response = await client.get(
        f"/api/v1/extraction-batches/{batch['id']}/candidates",
        headers=headers,
        params={"limit": 100},
    )
    assert candidate_response.status_code == 200
    return headers, episode, input_version, candidate_response.json()["data"]["items"]


async def _decide(
    client: httpx.AsyncClient,
    headers: dict[str, str],
    candidate: dict[str, Any],
    decision: dict[str, Any],
) -> None:
    response = await client.post(
        f"/api/v1/extraction-candidates/{candidate['id']}/decisions",
        headers=headers,
        json={
            "decision_key": f"decision-{candidate['candidate_key']}",
            "expected_revision": candidate["revision"],
            "decision": decision,
        },
    )
    assert response.status_code == 201


@pytest.mark.asyncio
async def test_confirm_structure_lists_unresolved_required_and_blocking_candidates(
    client: httpx.AsyncClient,
    session_factory: async_sessionmaker[AsyncSession],
) -> None:
    headers, _, input_version, candidates = await _completed_batch(
        client,
        session_factory,
        email="structure-blocked@example.com",
        key="structure-blocked",
        candidates=_blocking_candidates(),
    )
    by_key = {candidate["candidate_key"]: candidate for candidate in candidates}
    await _decide(
        client,
        headers,
        by_key["scene-001"],
        {"action": "accept_new"},
    )

    response = await client.post(
        f"/api/v1/extraction-batches/{by_key['scene-001']['batch_id']}/confirm-structure",
        headers=headers,
    )
    assert response.status_code == 409
    error = response.json()["error"]
    assert error["code"] == "state_conflict"
    assert error["next_action"] == "resolve_structure_candidates"
    assert [item["candidate_key"] for item in error["details"]["unresolved_candidates"]] == [
        "dialogue-001"
    ]
    assert [
        item["candidate_key"]
        for item in error["details"]["blocking_continuity_candidates"]
    ] == ["continuity-001"]

    async with session_factory() as session:
        version_table = Base.metadata.tables["scr_script_versions"]
        scene_table = Base.metadata.tables["scr_scenes"]
        dialogue_table = Base.metadata.tables["scr_dialogues"]
        assert (
            await session.scalar(
                select(func.count())
                .select_from(version_table)
                .where(version_table.c.source_id == UUID(input_version["source_id"]))
            )
            == 2
        )
        assert await session.scalar(select(func.count()).select_from(scene_table)) == 0
        assert await session.scalar(select(func.count()).select_from(dialogue_table)) == 0


@pytest.mark.asyncio
async def test_confirm_structure_is_concurrent_idempotent_ordered_and_private(
    client: httpx.AsyncClient,
    session_factory: async_sessionmaker[AsyncSession],
) -> None:
    headers, episode, input_version, candidates = await _completed_batch(
        client,
        session_factory,
        email="structure-success@example.com",
        key="structure-success",
        candidates=_ordered_candidates(),
    )
    by_key = {candidate["candidate_key"]: candidate for candidate in candidates}
    changed_scene = {
        **by_key["scene-early"]["proposal"],
        "summary": "人工确认后的第一场",
    }
    await _decide(
        client,
        headers,
        by_key["scene-early"],
        {"action": "accept_with_changes", "proposal": changed_scene},
    )
    await _decide(client, headers, by_key["scene-late"], {"action": "accept_new"})
    await _decide(
        client,
        headers,
        by_key["scene-alias"],
        {
            "action": "merge_into",
            "target_candidate_id": by_key["scene-early"]["id"],
        },
    )
    await _decide(
        client,
        headers,
        by_key["dialogue-alias"],
        {"action": "accept_new"},
    )
    await _decide(
        client,
        headers,
        by_key["dialogue-ignore"],
        {"action": "ignore"},
    )
    batch_id = by_key["scene-early"]["batch_id"]
    endpoint = f"/api/v1/extraction-batches/{batch_id}/confirm-structure"

    first, second = await asyncio.gather(
        client.post(endpoint, headers=headers),
        client.post(endpoint, headers=headers),
    )
    assert first.status_code == second.status_code == 201
    assert first.json()["data"] == second.json()["data"]
    confirmation = first.json()["data"]
    confirmed_version = confirmation["confirmed_version"]
    assert confirmation["batch_id"] == batch_id
    assert confirmation["source_script_version_id"] == input_version["id"]
    assert confirmed_version["id"] != input_version["id"]
    assert confirmed_version["version_no"] == input_version["version_no"] + 1
    assert confirmed_version["status"] == "published"
    assert confirmed_version["body"] == SCRIPT_BODY

    scenes = confirmation["scenes"]
    assert [(scene["position"], scene["heading"]) for scene in scenes] == [
        (1, "第一场"),
        (2, "第二场"),
    ]
    assert scenes[0]["summary"] == "人工确认后的第一场"
    assert scenes[0]["source_range"] == {"start": 0, "end": 10}
    assert len(scenes[0]["dialogues"]) == 1
    dialogue = scenes[0]["dialogues"][0]
    assert dialogue["position"] == 1
    assert dialogue["speaker_candidate"] == "角色甲"
    assert dialogue["text"] == "开始。"
    assert dialogue["source_range"] == {"start": 21, "end": 30}
    assert scenes[1]["dialogues"] == []

    batch_response = await client.get(
        f"/api/v1/extraction-batches/{batch_id}", headers=headers
    )
    assert batch_response.status_code == 200
    assert (
        batch_response.json()["data"]["confirmed_script_version_id"]
        == confirmed_version["id"]
    )
    confirmed_delete = await client.delete(
        f"/api/v1/script-versions/{confirmed_version['id']}",
        headers=headers,
        params={"confirm": "true"},
    )
    assert confirmed_delete.status_code == 409
    assert [
        blocker["code"]
        for blocker in confirmed_delete.json()["error"]["details"]["blockers"]
    ] == ["VERSION_NOT_DRAFT", "CONFIRMED_STRUCTURE_VERSION"]
    stranger = await register_identity_response(
        client,
        email="structure-stranger@example.com",
        password="a-secure-structure-password",
        display_name="其他用户",
    )
    stranger_headers = {
        "authorization": f"Bearer {stranger.json()['data']['access_token']}"
    }
    assert (await client.post(endpoint, headers=stranger_headers)).status_code == 404

    async with session_factory() as session:
        version_table = Base.metadata.tables["scr_script_versions"]
        scene_table = Base.metadata.tables["scr_scenes"]
        dialogue_table = Base.metadata.tables["scr_dialogues"]
        assert (
            await session.scalar(
                select(func.count())
                .select_from(version_table)
                .where(version_table.c.source_id == UUID(input_version["source_id"]))
            )
            == 3
        )
        assert await session.scalar(select(func.count()).select_from(scene_table)) == 2
        assert await session.scalar(select(func.count()).select_from(dialogue_table)) == 1
        stored_episode = await session.get(Episode, UUID(episode["id"]))
        assert stored_episode is not None
        assert str(stored_episode.current_script_version_id) == input_version["id"]


@pytest.mark.asyncio
async def test_confirmed_asset_candidates_create_or_link_idempotently(
    client: httpx.AsyncClient,
    session_factory: async_sessionmaker[AsyncSession],
) -> None:
    candidates = _ordered_candidates()
    candidates.append(
        _candidate(
            "asset-link",
            81,
            90,
            {
                "kind": "asset",
                "asset_kind": "character",
                "name": "角色甲别名",
                "description": "关联同一个角色身份",
            },
        )
    )
    headers, episode, _, rows = await _completed_batch(
        client,
        session_factory,
        email="asset-handoff@example.com",
        key="asset-handoff",
        candidates=candidates,
    )
    by_key = {candidate["candidate_key"]: candidate for candidate in rows}
    await _decide(client, headers, by_key["scene-early"], {"action": "accept_new"})
    await _decide(client, headers, by_key["scene-late"], {"action": "accept_new"})
    await _decide(
        client,
        headers,
        by_key["scene-alias"],
        {
            "action": "merge_into",
            "target_candidate_id": by_key["scene-early"]["id"],
        },
    )
    await _decide(client, headers, by_key["dialogue-alias"], {"action": "accept_new"})
    await _decide(client, headers, by_key["dialogue-ignore"], {"action": "ignore"})
    confirmed = await client.post(
        f"/api/v1/extraction-batches/{by_key['scene-early']['batch_id']}/confirm-structure",
        headers=headers,
    )
    assert confirmed.status_code == 201

    create_payload = {
        "decision_key": "create-asset-from-candidate",
        "expected_revision": 1,
        "decision": {"action": "accept_new"},
    }
    created = await client.post(
        f"/api/v1/extraction-candidates/{by_key['asset-pending']['id']}/decisions",
        headers=headers,
        json=create_payload,
    )
    assert created.status_code == 201
    created_data = created.json()["data"]
    assert created_data["candidate"]["status"] == "accepted"
    assert created_data["evidence"]["downstream_type"] == "ASSET"
    asset_id = created_data["evidence"]["downstream_id"]
    assert asset_id

    replay = await client.post(
        f"/api/v1/extraction-candidates/{by_key['asset-pending']['id']}/decisions",
        headers=headers,
        json=create_payload,
    )
    assert replay.status_code == 201
    assert replay.json()["data"] == created_data

    linked = await client.post(
        f"/api/v1/extraction-candidates/{by_key['asset-link']['id']}/decisions",
        headers=headers,
        json={
            "decision_key": "link-existing-asset",
            "expected_revision": 1,
            "decision": {
                "action": "link_existing",
                "downstream_id": asset_id,
            },
        },
    )
    assert linked.status_code == 201
    linked_data = linked.json()["data"]
    assert linked_data["candidate"]["status"] == "linked"
    assert linked_data["evidence"]["downstream_id"] == asset_id

    detail = await client.get(f"/api/v1/assets/{asset_id}", headers=headers)
    assert detail.status_code == 200
    assert detail.json()["data"]["project_id"] == episode["project_id"]
    assert detail.json()["data"]["current_version_id"]

    async with session_factory() as session:
        assert await session.scalar(select(func.count()).select_from(Asset)) == 1
        assert await session.scalar(select(func.count()).select_from(AssetVersion)) == 1
