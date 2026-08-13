import asyncio
from collections.abc import Awaitable
from copy import deepcopy
from datetime import UTC, datetime, timedelta
from typing import Any, Protocol, cast
from uuid import UUID

import httpx
import pytest
from sqlalchemy import event, select
from sqlalchemy.ext.asyncio import AsyncEngine, AsyncSession, async_sessionmaker
from uuid6 import uuid7

from app.modules.projects.models import Episode
from app.modules.scripts.models import (
    CandidateDecision,
    Dialogue,
    ExtractionBatch,
    ExtractionCandidate,
    Scene,
    ScriptSource,
    ScriptVersion,
)
from app.modules.scripts.narratives.service import record_current_impact
from app.modules.storyboards.hashing import storyboard_content_hashes
from app.modules.storyboards.models import AssetReference, Shot, ShotSpecVersion
from app.modules.storyboards.schemas import AssetReferenceRequest, ShotSpec
from tests.support.identity_builders import register_identity_response
from tests.support.media_builders import seed_ready_media_version
from tests.support.project_builders import project_payload


class _SnapshotReader(Protocol):
    def __call__(
        self,
        session: AsyncSession,
        workspace_id: UUID,
        version_id: UUID,
    ) -> Awaitable[Any]: ...


async def create_episode_with_confirmed_structure(
    client: httpx.AsyncClient,
    session_factory: async_sessionmaker[AsyncSession],
    *,
    email: str,
) -> tuple[dict[str, str], dict[str, Any], dict[str, UUID]]:
    registered = await register_identity_response(client, email=email)
    assert registered.status_code == 201
    identity = registered.json()["data"]
    headers = {"authorization": f"Bearer {identity['access_token']}"}
    created_project = await client.post(
        "/api/v1/projects",
        headers=headers,
        json=project_payload(identity["workspace"]["id"], "分镜验收项目"),
    )
    assert created_project.status_code == 201
    project = created_project.json()["data"]
    created_episode = await client.post(
        f"/api/v1/projects/{project['id']}/episodes",
        headers=headers,
        json={"name": "第一集", "target_duration_ms": 90_000},
    )
    assert created_episode.status_code == 201
    episode = created_episode.json()["data"]

    workspace_id = UUID(identity["workspace"]["id"])
    actor_id = UUID(identity["user"]["id"])
    episode_id = UUID(episode["id"])
    source_id = uuid7()
    script_version_id = uuid7()
    scene_id = uuid7()
    dialogue_id = uuid7()
    async with session_factory() as session, session.begin():
        source = ScriptSource(
            id=source_id,
            workspace_id=workspace_id,
            episode_id=episode_id,
            input_type="text",
            title="已确认测试剧本",
            rights_declaration="虚构测试文本",
            status="active",
            revision=1,
            idempotency_key=f"confirmed-{episode_id}",
        )
        version = ScriptVersion(
            id=script_version_id,
            workspace_id=workspace_id,
            source_id=source_id,
            version_no=1,
            status="published",
            body="雨夜车站\n林澈：有人吗？",
            content_hash="1" * 64,
            structure_summary={
                "confirmation_batch_id": str(uuid7()),
                "source_script_version_id": str(uuid7()),
                "scene_count": 1,
                "dialogue_count": 1,
            },
            created_by=actor_id,
        )
        scene = Scene(
            id=scene_id,
            workspace_id=workspace_id,
            script_version_id=script_version_id,
            position=1,
            heading="雨夜车站",
            location="旧车站月台",
            time_of_day="夜",
            summary="林澈进入空无一人的月台",
            source_start=0,
            source_end=10,
        )
        dialogue = Dialogue(
            id=dialogue_id,
            workspace_id=workspace_id,
            scene_id=scene_id,
            position=1,
            speaker_candidate="林澈",
            dialogue_kind="spoken",
            text="有人吗？",
            source_start=11,
            source_end=16,
        )
        persisted_episode = await session.scalar(
            select(Episode).where(Episode.id == episode_id).with_for_update()
        )
        assert persisted_episode is not None
        session.add(source)
        await session.flush()
        session.add(version)
        await session.flush()
        session.add(scene)
        await session.flush()
        session.add(dialogue)
        persisted_episode.current_script_version_id = script_version_id
        persisted_episode.revision += 1
        await session.flush()
        await record_current_impact(
            session,
            workspace_id=workspace_id,
            episode_id=episode_id,
            episode_revision=persisted_episode.revision,
            previous_script_version_id=None,
            current_script_version_id=script_version_id,
            affected_shot_ids=[],
            actor_id=actor_id,
        )

    return (
        headers,
        episode,
        {
            "workspace_id": workspace_id,
            "actor_id": actor_id,
            "episode_id": episode_id,
            "script_version_id": script_version_id,
            "scene_id": scene_id,
            "dialogue_id": dialogue_id,
        },
    )


def shot_creation_payload(
    refs: dict[str, UUID],
    *,
    title: str,
    creation_key: str,
) -> dict[str, str]:
    return {
        "title": title,
        "source_script_version_id": str(refs["script_version_id"]),
        "source_scene_id": str(refs["scene_id"]),
        "creation_key": creation_key,
    }


def shot_spec_payload(refs: dict[str, UUID], *, purpose: str) -> dict[str, object]:
    return {
        "schema_version": 1,
        "script_reference": {
            "confirmed_script_version_id": str(refs["script_version_id"]),
            "scene_id": str(refs["scene_id"]),
            "dialogue_ids": [str(refs["dialogue_id"])],
        },
        "narrative": {"purpose": purpose, "continuity_note": None},
        "visual": {
            "shot_size": "medium",
            "camera_angle": "eye_level",
            "camera_movement": "static",
            "composition": "林澈位于画面中心",
            "environment": "雨夜旧车站月台",
            "subject_placements": [{"subject_key": "hero", "placement": "画面中心"}],
            "mood_lighting": "冷蓝顶光",
        },
        "action_beats": [{"beat_key": "pause", "order": 1, "description": "林澈停下脚步"}],
        "dialogue_or_narration": [
            {
                "source_dialogue_id": str(refs["dialogue_id"]),
                "beat_key": "pause",
                "speaker_subject_key": "hero",
                "render_as_audio": True,
                "performance_note": "保持警惕",
            }
        ],
        "duration_ms": 3000,
        "audio_intent": {"ambient": "雨声", "sound_effects": []},
        "generation_intent": {
            "mode": "text_to_video",
            "first_frame": None,
            "last_frame": None,
            "keyframe_notes": None,
        },
    }


async def _asset_version_consent(
    client: httpx.AsyncClient,
    *,
    headers: dict[str, str],
    refs: dict[str, UUID],
    version_id: UUID,
    proof_media_version_id: UUID,
    idempotency_key: str,
) -> dict[str, Any]:
    now = datetime.now(UTC)
    consent_response = await client.post(
        "/api/v1/consents",
        headers=headers,
        json={
            "workspace_id": str(refs["workspace_id"]),
            "subject_identity": {
                "reference": f"synthetic-storyboard-asset-{version_id}",
                "kind": "fictional_adult",
            },
            "scope": {
                "type": "media_usage",
                "subject_type": "ASSET_VERSION",
                "subject_id": str(version_id),
                "rights_holder_role": "synthetic_creator",
                "rights_types": ["copyright", "image"],
                "authorized_purposes": ["ai_short_drama_generation"],
                "channels": ["lanverse_preview"],
                "regions": ["CN"],
                "valid_from": (now - timedelta(days=1)).isoformat(),
                "valid_to": (now + timedelta(days=365)).isoformat(),
            },
            "proof_media_version_ids": [str(proof_media_version_id)],
            "reason": "分镜准备度资产授权验收",
            "idempotency_key": idempotency_key,
        },
    )
    assert consent_response.status_code == 201
    return consent_response.json()["data"]


async def create_ready_location_asset(
    client: httpx.AsyncClient,
    session_factory: async_sessionmaker[AsyncSession],
    *,
    headers: dict[str, str],
    project_id: UUID,
    refs: dict[str, UUID],
) -> tuple[dict[str, Any], dict[str, Any]]:
    media_version_id = await seed_ready_media_version(
        session_factory,
        workspace_id=refs["workspace_id"],
        actor_id=refs["actor_id"],
        kind="image",
        filename="storyboard-location.png",
        mime_type="image/png",
    )
    asset_response = await client.post(
        f"/api/v1/projects/{project_id}/assets",
        headers=headers,
        json={
            "kind": "location",
            "name": "雨夜旧车站",
            "aliases": [],
            "tags": ["分镜验收"],
        },
    )
    assert asset_response.status_code == 201
    asset = asset_response.json()["data"]
    version_response = await client.post(
        f"/api/v1/assets/{asset['id']}/versions",
        headers=headers,
        json={
            "spec": {
                "kind": "location",
                "spatial_description": "封闭的旧车站月台",
                "time_weather": "雨夜",
                "visual_elements": ["旧灯箱", "积水"],
                "lighting": "冷蓝顶光",
            },
            "prompt_description": "固定雨夜旧车站空间和光线",
            "media_references": [
                {
                    "media_version_id": str(media_version_id),
                    "purpose": "environment",
                    "position": 1,
                }
            ],
            "source_type": "manual",
            "source_id": None,
            "expected_current_version_id": None,
            "set_as_current": True,
        },
    )
    assert version_response.status_code == 201
    version = version_response.json()["data"]["version"]
    consent = await _asset_version_consent(
        client,
        headers=headers,
        refs=refs,
        version_id=UUID(version["id"]),
        proof_media_version_id=media_version_id,
        idempotency_key=f"storyboard-location-consent-{version['id']}",
    )
    return version, consent


async def _append_ready_location_version(
    client: httpx.AsyncClient,
    *,
    headers: dict[str, str],
    refs: dict[str, UUID],
    current_version: dict[str, Any],
) -> dict[str, Any]:
    media_version_id = UUID(current_version["media_references"][0]["media_version_id"])
    response = await client.post(
        f"/api/v1/assets/{current_version['asset_id']}/versions",
        headers=headers,
        json={
            "spec": {
                "kind": "location",
                "spatial_description": "封闭的旧车站月台",
                "time_weather": "暴雨深夜",
                "visual_elements": ["修复后的旧灯箱", "积水"],
                "lighting": "更新后的冷蓝侧逆光",
            },
            "prompt_description": "固定升级后的雨夜旧车站空间和光线",
            "media_references": [
                {
                    "media_version_id": str(media_version_id),
                    "purpose": "environment",
                    "position": 1,
                }
            ],
            "source_type": "manual",
            "source_id": None,
            "expected_current_version_id": current_version["id"],
            "set_as_current": True,
        },
    )
    assert response.status_code == 201
    version = response.json()["data"]["version"]
    await _asset_version_consent(
        client,
        headers=headers,
        refs=refs,
        version_id=UUID(version["id"]),
        proof_media_version_id=media_version_id,
        idempotency_key=f"storyboard-upgrade-consent-{version['id']}",
    )
    return version


async def _seed_ready_storyboard_shots(
    session_factory: async_sessionmaker[AsyncSession],
    *,
    refs: dict[str, UUID],
    location_version_id: UUID,
    count: int,
) -> None:
    spec_payload = deepcopy(shot_spec_payload(refs, purpose="批量准备度性能基线"))
    visual = dict(cast(dict[str, object], spec_payload["visual"]))
    visual["subject_placements"] = []
    spec_payload["visual"] = visual
    spec_payload["dialogue_or_narration"] = []
    spec = ShotSpec.model_validate(spec_payload)
    reference_request = AssetReferenceRequest(
        slot_key="location-main",
        role="location",
        asset_version_id=location_version_id,
        subject_key=None,
    )
    hashes = storyboard_content_hashes(spec, [reference_request])
    async with session_factory() as session, session.begin():
        shots: list[Shot] = []
        versions: list[ShotSpecVersion] = []
        references: list[AssetReference] = []
        for position in range(1, count + 1):
            shot_id = uuid7()
            spec_version_id = uuid7()
            shots.append(
                Shot(
                    id=shot_id,
                    workspace_id=refs["workspace_id"],
                    episode_id=refs["episode_id"],
                    position=position,
                    title=f"批量准备度镜头 {position:03d}",
                    source_script_version_id=refs["script_version_id"],
                    source_scene_id=refs["scene_id"],
                    source_candidate_id=None,
                    creation_key=f"batch-readiness-{position:03d}",
                    status="active",
                    current_spec_version_id=spec_version_id,
                    revision=1,
                    created_by=refs["actor_id"],
                )
            )
            versions.append(
                ShotSpecVersion(
                    id=spec_version_id,
                    workspace_id=refs["workspace_id"],
                    shot_id=shot_id,
                    version_no=1,
                    schema_version=1,
                    spec=spec.model_dump(mode="json"),
                    content_hash=hashes.content_hash,
                    input_hash=hashes.input_hash,
                    created_by=refs["actor_id"],
                )
            )
            references.append(
                AssetReference(
                    id=uuid7(),
                    workspace_id=refs["workspace_id"],
                    shot_spec_version_id=spec_version_id,
                    slot_key="location-main",
                    role="location",
                    asset_version_id=location_version_id,
                    subject_key=None,
                )
            )
        session.add_all(shots)
        await session.flush()
        session.add_all(versions)
        await session.flush()
        session.add_all(references)


async def _seed_confirmed_shot_candidate(
    session_factory: async_sessionmaker[AsyncSession],
    refs: dict[str, UUID],
) -> UUID:
    batch_id = uuid7()
    scene_candidate_id = uuid7()
    shot_candidate_id = uuid7()
    async with session_factory() as session, session.begin():
        batch = ExtractionBatch(
            id=batch_id,
            workspace_id=refs["workspace_id"],
            script_version_id=refs["script_version_id"],
            scope="full",
            extractor_version="test-confirmed-structure",
            input_hash="2" * 64,
            status="succeeded",
            confirmed_script_version_id=refs["script_version_id"],
            candidate_count=2,
            idempotency_key=f"confirmed-shot-{batch_id}",
            created_by=refs["actor_id"],
        )
        session.add(batch)
        await session.flush()
        scene_candidate = ExtractionCandidate(
            id=scene_candidate_id,
            workspace_id=refs["workspace_id"],
            batch_id=batch_id,
            candidate_key="scene-001",
            kind="scene",
            source_start=0,
            source_end=10,
            proposal={
                "kind": "scene",
                "heading": "雨夜车站",
                "location": "旧车站月台",
                "time_of_day": "夜",
                "summary": "林澈进入空无一人的月台",
            },
            required=True,
            status="accepted",
            revision=2,
        )
        shot_candidate = ExtractionCandidate(
            id=shot_candidate_id,
            workspace_id=refs["workspace_id"],
            batch_id=batch_id,
            candidate_key="shot-001",
            kind="shot",
            source_start=0,
            source_end=10,
            proposal={
                "kind": "shot",
                "scene_candidate_key": "scene-001",
                "title": "AI 候选：进入车站",
                "purpose": "建立雨夜车站环境与人物状态",
            },
            required=False,
            status="accepted",
            revision=2,
        )
        session.add_all([scene_candidate, shot_candidate])
        await session.flush()
        session.add_all(
            [
                CandidateDecision(
                    id=uuid7(),
                    workspace_id=refs["workspace_id"],
                    candidate_id=scene_candidate_id,
                    sequence=1,
                    decision_key="accept-scene",
                    action="accept_new",
                    payload={},
                    actor_id=refs["actor_id"],
                ),
                CandidateDecision(
                    id=uuid7(),
                    workspace_id=refs["workspace_id"],
                    candidate_id=shot_candidate_id,
                    sequence=1,
                    decision_key="accept-shot",
                    action="accept_new",
                    payload={},
                    actor_id=refs["actor_id"],
                ),
            ]
        )
    return shot_candidate_id


@pytest.mark.asyncio
async def test_manual_shot_is_idempotent_ordered_and_lifecycle_safe(
    client: httpx.AsyncClient,
    session_factory: async_sessionmaker[AsyncSession],
) -> None:
    headers, episode, refs = await create_episode_with_confirmed_structure(
        client,
        session_factory,
        email="storyboard-lifecycle@example.com",
    )
    endpoint = f"/api/v1/episodes/{episode['id']}/shots"
    first_payload = shot_creation_payload(
        refs,
        title="进入车站",
        creation_key="manual-shot-001",
    )
    first_response = await client.post(endpoint, headers=headers, json=first_payload)
    assert first_response.status_code == 201
    first = first_response.json()["data"]
    assert first["position"] == 1
    assert first["status"] == "active"
    assert first["current_spec_version_id"] is None
    assert first["revision"] == 1

    repeated = await client.post(endpoint, headers=headers, json=first_payload)
    assert repeated.status_code == 201
    assert repeated.json()["data"] == first
    conflicting = await client.post(
        endpoint,
        headers=headers,
        json=first_payload | {"title": "同一个键不能改变输入"},
    )
    assert conflicting.status_code == 409
    assert conflicting.json()["error"]["code"] == "resource_conflict"

    second_response = await client.post(
        endpoint,
        headers=headers,
        json=shot_creation_payload(
            refs,
            title="发现灯箱",
            creation_key="manual-shot-002",
        ),
    )
    assert second_response.status_code == 201
    second = second_response.json()["data"]
    assert second["position"] == 2

    renamed_response = await client.patch(
        f"/api/v1/shots/{first['id']}",
        headers=headers,
        json={
            "expected_revision": first["revision"],
            "title": "进入雨夜车站",
        },
    )
    assert renamed_response.status_code == 200
    first = renamed_response.json()["data"]
    assert first["title"] == "进入雨夜车站"
    assert first["revision"] == 2

    stale_rename = await client.patch(
        f"/api/v1/shots/{first['id']}",
        headers=headers,
        json={
            "expected_revision": 1,
            "title": "陈旧标题不能覆盖",
        },
    )
    assert stale_rename.status_code == 409
    assert stale_rename.json()["error"]["code"] == "version_conflict"
    assert stale_rename.json()["error"]["details"]["current_revision"] == 2

    listed = await client.get(endpoint, headers=headers)
    assert listed.status_code == 200
    order = listed.json()["data"]
    assert [item["id"] for item in order["items"]] == [first["id"], second["id"]]
    assert len(order["order_hash"]) == 64

    reordered = await client.post(
        f"{endpoint}/reorder",
        headers=headers,
        json={
            "shot_ids": [second["id"], first["id"]],
            "expected_order_hash": order["order_hash"],
        },
    )
    assert reordered.status_code == 200
    new_order = reordered.json()["data"]
    assert [item["id"] for item in new_order["items"]] == [second["id"], first["id"]]
    assert new_order["order_hash"] != order["order_hash"]

    stale = await client.post(
        f"{endpoint}/reorder",
        headers=headers,
        json={
            "shot_ids": [first["id"], second["id"]],
            "expected_order_hash": order["order_hash"],
        },
    )
    assert stale.status_code == 409
    assert stale.json()["error"]["code"] == "version_conflict"
    assert stale.json()["error"]["details"]["current_order_hash"] == new_order["order_hash"]

    archived = await client.post(
        f"/api/v1/shots/{first['id']}/archive",
        headers=headers,
        json={
            "expected_revision": first["revision"],
            "expected_order_hash": new_order["order_hash"],
        },
    )
    assert archived.status_code == 200
    assert archived.json()["data"]["shot"]["status"] == "archived"
    assert [item["id"] for item in archived.json()["data"]["order"]["items"]] == [second["id"]]

    archived_list = await client.get(
        f"/api/v1/episodes/{episode['id']}/archived-shots",
        headers=headers,
    )
    assert archived_list.status_code == 200
    assert [item["id"] for item in archived_list.json()["data"]] == [first["id"]]

    restored = await client.post(
        f"/api/v1/shots/{first['id']}/restore",
        headers=headers,
        json={
            "expected_revision": archived.json()["data"]["shot"]["revision"],
            "expected_order_hash": archived.json()["data"]["order"]["order_hash"],
        },
    )
    assert restored.status_code == 200
    assert restored.json()["data"]["shot"]["position"] == 2
    assert (
        await client.get(
            f"/api/v1/episodes/{episode['id']}/archived-shots",
            headers=headers,
        )
    ).json()["data"] == []


@pytest.mark.asyncio
async def test_storyboard_facts_are_itemized_in_episode_and_project_delete_guards(
    client: httpx.AsyncClient,
    session_factory: async_sessionmaker[AsyncSession],
) -> None:
    headers, episode, refs = await create_episode_with_confirmed_structure(
        client,
        session_factory,
        email="storyboard-delete-blocker@example.com",
    )
    shots_endpoint = f"/api/v1/episodes/{episode['id']}/shots"
    first_response = await client.post(
        shots_endpoint,
        headers=headers,
        json=shot_creation_payload(
            refs,
            title="保留规格历史的镜头",
            creation_key="storyboard-delete-first",
        ),
    )
    second_response = await client.post(
        shots_endpoint,
        headers=headers,
        json=shot_creation_payload(
            refs,
            title="归档后仍需保留的镜头",
            creation_key="storyboard-delete-second",
        ),
    )
    assert first_response.status_code == 201
    assert second_response.status_code == 201
    first = first_response.json()["data"]
    second = second_response.json()["data"]

    first_spec = await client.post(
        f"/api/v1/shots/{first['id']}/spec-versions",
        headers=headers,
        json={
            "expected_current_spec_version_id": None,
            "spec": shot_spec_payload(refs, purpose="固定第一版制作规格"),
            "asset_references": [],
        },
    )
    assert first_spec.status_code == 201
    second_spec = await client.post(
        f"/api/v1/shots/{first['id']}/spec-versions",
        headers=headers,
        json={
            "expected_current_spec_version_id": first_spec.json()["data"]["version"]["id"],
            "spec": shot_spec_payload(refs, purpose="保留不可变历史规格"),
            "asset_references": [],
        },
    )
    assert second_spec.status_code == 201

    order = await client.get(shots_endpoint, headers=headers)
    assert order.status_code == 200
    archived = await client.post(
        f"/api/v1/shots/{second['id']}/archive",
        headers=headers,
        json={
            "expected_revision": second["revision"],
            "expected_order_hash": order.json()["data"]["order_hash"],
        },
    )
    assert archived.status_code == 200
    assert archived.json()["data"]["shot"]["status"] == "archived"

    episode_preflight = await client.post(
        f"/api/v1/episodes/{episode['id']}/delete-preflight",
        headers=headers,
    )
    assert episode_preflight.status_code == 200
    assert {
        blocker["code"]: blocker["summary"]
        for blocker in episode_preflight.json()["data"]["blockers"]
    } == {
        "HAS_SCRIPT_VERSIONS": "单集已有 1 个剧本版本",
        "HAS_STORYBOARD_SHOTS": "单集已有 2 个分镜镜头（2 个规格版本）",
    }

    project_preflight = await client.post(
        f"/api/v1/projects/{episode['project_id']}/delete-preflight",
        headers=headers,
    )
    assert project_preflight.status_code == 200
    assert {
        blocker["code"]: blocker["summary"]
        for blocker in project_preflight.json()["data"]["blockers"]
    } == {
        "HAS_EPISODES": "项目包含 1 个单集",
        "HAS_SCRIPT_VERSIONS": "项目关联 1 个剧本版本",
        "HAS_STORYBOARD_SHOTS": "项目关联 2 个分镜镜头（2 个规格版本）",
    }

    current_episode = await client.get(f"/api/v1/episodes/{episode['id']}", headers=headers)
    assert current_episode.status_code == 200
    blocked_delete = await client.delete(
        f"/api/v1/episodes/{episode['id']}",
        headers=headers,
        params={"expected_revision": current_episode.json()["data"]["revision"]},
    )
    assert blocked_delete.status_code == 409
    assert blocked_delete.json()["error"]["code"] == "state_conflict"
    assert blocked_delete.json()["error"]["message"] == "Episode has dependent storyboard facts"
    assert blocked_delete.json()["error"]["next_action"] == "review_delete_blockers"
    assert (
        await client.get(
            f"/api/v1/shot-spec-versions/{first_spec.json()['data']['version']['id']}",
            headers=headers,
        )
    ).status_code == 200


@pytest.mark.asyncio
async def test_concurrent_first_shot_creation_and_episode_delete_are_serialized(
    client: httpx.AsyncClient,
    session_factory: async_sessionmaker[AsyncSession],
) -> None:
    headers, episode, refs = await create_episode_with_confirmed_structure(
        client,
        session_factory,
        email="storyboard-delete-race@example.com",
    )
    current_episode = await client.get(f"/api/v1/episodes/{episode['id']}", headers=headers)
    assert current_episode.status_code == 200

    create_response, delete_response = await asyncio.gather(
        client.post(
            f"/api/v1/episodes/{episode['id']}/shots",
            headers=headers,
            json=shot_creation_payload(
                refs,
                title="并发创建镜头",
                creation_key="storyboard-delete-race",
            ),
        ),
        client.delete(
            f"/api/v1/episodes/{episode['id']}",
            headers=headers,
            params={"expected_revision": current_episode.json()["data"]["revision"]},
        ),
    )

    assert create_response.status_code == 201
    assert delete_response.status_code == 409
    assert delete_response.json()["error"]["message"] in {
        "Episode has dependent storyboard facts",
        "Episode has dependent script versions",
    }
    listed = await client.get(f"/api/v1/episodes/{episode['id']}/shots", headers=headers)
    assert listed.status_code == 200
    assert [item["id"] for item in listed.json()["data"]["items"]] == [
        create_response.json()["data"]["id"]
    ]


@pytest.mark.asyncio
async def test_shot_spec_versions_are_immutable_and_compare_current_pointer(
    client: httpx.AsyncClient,
    session_factory: async_sessionmaker[AsyncSession],
) -> None:
    headers, episode, refs = await create_episode_with_confirmed_structure(
        client,
        session_factory,
        email="storyboard-spec@example.com",
    )
    created = await client.post(
        f"/api/v1/episodes/{episode['id']}/shots",
        headers=headers,
        json=shot_creation_payload(
            refs,
            title="规格版本镜头",
            creation_key="manual-spec-shot",
        ),
    )
    assert created.status_code == 201
    shot = created.json()["data"]
    versions_endpoint = f"/api/v1/shots/{shot['id']}/spec-versions"

    first_response = await client.post(
        versions_endpoint,
        headers=headers,
        json={
            "expected_current_spec_version_id": None,
            "spec": shot_spec_payload(refs, purpose="交代主角停下观察"),
            "asset_references": [],
        },
    )
    assert first_response.status_code == 201
    first = first_response.json()["data"]
    assert first["version"]["version_no"] == 1
    assert first["shot"]["current_spec_version_id"] == first["version"]["id"]
    assert len(first["version"]["content_hash"]) == 64

    conflict = await client.post(
        versions_endpoint,
        headers=headers,
        json={
            "expected_current_spec_version_id": None,
            "spec": shot_spec_payload(refs, purpose="不能覆盖并发版本"),
            "asset_references": [],
        },
    )
    assert conflict.status_code == 409
    assert conflict.json()["error"]["code"] == "version_conflict"
    assert conflict.json()["error"]["details"]["current_spec_version_id"] == first["version"]["id"]

    second_response = await client.post(
        versions_endpoint,
        headers=headers,
        json={
            "expected_current_spec_version_id": first["version"]["id"],
            "spec": shot_spec_payload(refs, purpose="强化主角的警觉反应"),
            "asset_references": [],
        },
    )
    assert second_response.status_code == 201
    second = second_response.json()["data"]
    assert second["version"]["version_no"] == 2
    assert second["version"]["id"] != first["version"]["id"]
    assert second["version"]["content_hash"] != first["version"]["content_hash"]

    history = await client.get(versions_endpoint, headers=headers)
    assert history.status_code == 200
    assert [item["id"] for item in history.json()["data"]] == [
        first["version"]["id"],
        second["version"]["id"],
    ]
    fetched_first = await client.get(
        f"/api/v1/shot-spec-versions/{first['version']['id']}",
        headers=headers,
    )
    assert fetched_first.status_code == 200
    assert fetched_first.json()["data"] == first["version"]

    switched = await client.post(
        f"/api/v1/shots/{shot['id']}/current-spec-version",
        headers=headers,
        json={
            "version_id": first["version"]["id"],
            "expected_current_spec_version_id": second["version"]["id"],
            "expected_revision": second["shot"]["revision"],
        },
    )
    assert switched.status_code == 200
    assert switched.json()["data"]["current_spec_version_id"] == first["version"]["id"]

    invalid_dialogue = shot_spec_payload(refs, purpose="引用不属于场景的对白")
    invalid_dialogue["script_reference"] = {
        "confirmed_script_version_id": str(refs["script_version_id"]),
        "scene_id": str(refs["scene_id"]),
        "dialogue_ids": [str(uuid7())],
    }
    invalid_dialogue["dialogue_or_narration"] = []
    rejected = await client.post(
        versions_endpoint,
        headers=headers,
        json={
            "expected_current_spec_version_id": first["version"]["id"],
            "spec": invalid_dialogue,
            "asset_references": [],
        },
    )
    assert rejected.status_code == 422
    assert rejected.json()["error"]["code"] == "validation_failed"

    version_audit = await client.get(
        "/api/v1/audit-events",
        headers=headers,
        params={
            "workspace_id": str(refs["workspace_id"]),
            "action": "shot.spec_version_created",
        },
    )
    assert version_audit.status_code == 200
    assert version_audit.json()["data"]["total"] == 2
    assert [
        item["metadata"]["version_no"] for item in reversed(version_audit.json()["data"]["items"])
    ] == [1, 2]
    assert all(
        item["metadata"]["source"] == "manual_save"
        for item in version_audit.json()["data"]["items"]
    )
    assert all(
        "spec" not in item["metadata"]
        and "content_hash" not in item["metadata"]
        and "input_hash" not in item["metadata"]
        for item in version_audit.json()["data"]["items"]
    )

    current_audit = await client.get(
        "/api/v1/audit-events",
        headers=headers,
        params={
            "workspace_id": str(refs["workspace_id"]),
            "action": "shot.current_spec_changed",
            "target_type": "shot",
            "target_id": shot["id"],
        },
    )
    assert current_audit.status_code == 200
    assert current_audit.json()["data"]["total"] == 1
    assert current_audit.json()["data"]["items"][0]["metadata"] == {
        "episode_id": episode["id"],
        "revision": switched.json()["data"]["revision"],
        "previous_version_id": second["version"]["id"],
        "current_version_id": first["version"]["id"],
    }


@pytest.mark.asyncio
async def test_confirmed_candidate_creation_and_safe_delete_preserve_evidence(
    client: httpx.AsyncClient,
    session_factory: async_sessionmaker[AsyncSession],
) -> None:
    headers, episode, refs = await create_episode_with_confirmed_structure(
        client,
        session_factory,
        email="storyboard-candidate-delete@example.com",
    )
    candidate_id = await _seed_confirmed_shot_candidate(session_factory, refs)

    candidate_created = await client.post(
        f"/api/v1/extraction-candidates/{candidate_id}/shot",
        headers=headers,
    )
    assert candidate_created.status_code == 201
    candidate_shot = candidate_created.json()["data"]
    assert candidate_shot["source_candidate_id"] == str(candidate_id)
    assert candidate_shot["title"] == "AI 候选：进入车站"
    repeated = await client.post(
        f"/api/v1/extraction-candidates/{candidate_id}/shot",
        headers=headers,
    )
    assert repeated.status_code == 201
    assert repeated.json()["data"] == candidate_shot

    candidate_preflight = await client.get(
        f"/api/v1/shots/{candidate_shot['id']}/delete-preflight",
        headers=headers,
    )
    assert candidate_preflight.status_code == 200
    assert candidate_preflight.json()["data"]["allowed"] is False
    assert [blocker["code"] for blocker in candidate_preflight.json()["data"]["blockers"]] == [
        "SOURCE_CANDIDATE_EVIDENCE"
    ]

    endpoint = f"/api/v1/episodes/{episode['id']}/shots"
    empty_response = await client.post(
        endpoint,
        headers=headers,
        json=shot_creation_payload(
            refs,
            title="可删除空镜头",
            creation_key="empty-delete-shot",
        ),
    )
    assert empty_response.status_code == 201
    empty = empty_response.json()["data"]
    listed = await client.get(endpoint, headers=headers)
    order = listed.json()["data"]

    empty_preflight = await client.get(
        f"/api/v1/shots/{empty['id']}/delete-preflight",
        headers=headers,
    )
    assert empty_preflight.status_code == 200
    assert empty_preflight.json()["data"] == {"allowed": True, "blockers": []}
    deleted = await client.delete(
        f"/api/v1/shots/{empty['id']}",
        headers=headers,
        params={
            "expected_revision": empty["revision"],
            "expected_order_hash": order["order_hash"],
        },
    )
    assert deleted.status_code == 200
    assert deleted.json()["data"]["deleted"] is True
    assert [item["id"] for item in deleted.json()["data"]["order"]["items"]] == [
        candidate_shot["id"]
    ]


def _split_target_spec(
    refs: dict[str, UUID],
    *,
    purpose: str,
    duration_ms: int,
    include_dialogue: bool,
) -> dict[str, object]:
    spec = deepcopy(shot_spec_payload(refs, purpose=purpose))
    spec["duration_ms"] = duration_ms
    if not include_dialogue:
        script_reference = dict(cast(dict[str, object], spec["script_reference"]))
        script_reference["dialogue_ids"] = []
        spec["script_reference"] = script_reference
        spec["dialogue_or_narration"] = []
    return spec


@pytest.mark.asyncio
async def test_copy_split_merge_are_atomic_idempotent_and_preserve_sources(
    client: httpx.AsyncClient,
    session_factory: async_sessionmaker[AsyncSession],
) -> None:
    headers, episode, refs = await create_episode_with_confirmed_structure(
        client,
        session_factory,
        email="storyboard-transform@example.com",
    )
    endpoint = f"/api/v1/episodes/{episode['id']}/shots"
    created = await client.post(
        endpoint,
        headers=headers,
        json=shot_creation_payload(
            refs,
            title="待变换镜头",
            creation_key="transform-source",
        ),
    )
    assert created.status_code == 201
    source = created.json()["data"]
    saved = await client.post(
        f"/api/v1/shots/{source['id']}/spec-versions",
        headers=headers,
        json={
            "expected_current_spec_version_id": None,
            "spec": shot_spec_payload(refs, purpose="建立车站悬疑氛围"),
            "asset_references": [],
        },
    )
    assert saved.status_code == 201
    source_spec = saved.json()["data"]["version"]
    initial_order = (await client.get(endpoint, headers=headers)).json()["data"]

    copy_payload = {
        "title": "待变换镜头副本",
        "expected_source_spec_version_id": source_spec["id"],
        "expected_order_hash": initial_order["order_hash"],
        "idempotency_key": "copy-transform-001",
    }
    copied_response = await client.post(
        f"/api/v1/shots/{source['id']}/copy",
        headers=headers,
        json=copy_payload,
    )
    assert copied_response.status_code == 201
    copied = copied_response.json()["data"]
    assert copied["transform"]["operation"] == "copy"
    assert copied["transform"]["source_shot_ids"] == [source["id"]]
    assert len(copied["shots"]) == 1
    assert copied["shots"][0]["source_candidate_id"] is None
    assert copied["spec_versions"][0]["content_hash"] == source_spec["content_hash"]
    assert [item["id"] for item in copied["order"]["items"]] == [
        source["id"],
        copied["shots"][0]["id"],
    ]
    repeated_copy = await client.post(
        f"/api/v1/shots/{source['id']}/copy",
        headers=headers,
        json=copy_payload,
    )
    assert repeated_copy.status_code == 201
    assert repeated_copy.json()["data"] == copied
    conflicting_copy = await client.post(
        f"/api/v1/shots/{source['id']}/copy",
        headers=headers,
        json=copy_payload | {"title": "同键不同输入"},
    )
    assert conflicting_copy.status_code == 409
    assert conflicting_copy.json()["error"]["code"] == "resource_conflict"

    split_preflight_payload = {
        "expected_source_spec_version_id": source_spec["id"],
        "expected_order_hash": copied["order"]["order_hash"],
    }
    split_preflight_response = await client.post(
        f"/api/v1/shots/{source['id']}/split-preflight",
        headers=headers,
        json=split_preflight_payload,
    )
    assert split_preflight_response.status_code == 200
    split_preflight = split_preflight_response.json()["data"]
    assert split_preflight["operation"] == "split"
    assert split_preflight["source_spec_version_ids"] == [source_spec["id"]]
    assert len(split_preflight["impact_hash"]) == 64
    assert split_preflight["downstream_evidence"] == {
        "generation_request_ids": [],
        "candidate_ids": [],
        "review_ids": [],
        "issue_ids": [],
        "timeline_source_ids": [],
    }

    split_payload: dict[str, object] = {
        **split_preflight_payload,
        "impact_hash": split_preflight["impact_hash"],
        "idempotency_key": "split-transform-001",
        "targets": [
            {
                "title": "进入月台",
                "spec": _split_target_spec(
                    refs,
                    purpose="主角进入月台并说话",
                    duration_ms=1500,
                    include_dialogue=True,
                ),
                "asset_references": [],
            },
            {
                "title": "观察灯箱",
                "spec": _split_target_spec(
                    refs,
                    purpose="主角观察异常灯箱",
                    duration_ms=1500,
                    include_dialogue=False,
                ),
                "asset_references": [],
            },
        ],
    }
    split_response = await client.post(
        f"/api/v1/shots/{source['id']}/split",
        headers=headers,
        json=split_payload,
    )
    assert split_response.status_code == 201
    split = split_response.json()["data"]
    assert split["transform"]["operation"] == "split"
    assert len(split["shots"]) == 2
    assert [item["position"] for item in split["shots"]] == [1, 2]
    archived_source = await client.get(f"/api/v1/shots/{source['id']}", headers=headers)
    assert archived_source.status_code == 200
    assert archived_source.json()["data"]["status"] == "archived"
    repeated_split = await client.post(
        f"/api/v1/shots/{source['id']}/split",
        headers=headers,
        json=split_payload,
    )
    assert repeated_split.status_code == 201
    assert repeated_split.json()["data"] == split

    merge_preflight_payload = {
        "shot_ids": [shot["id"] for shot in split["shots"]],
        "expected_spec_version_ids": [version["id"] for version in split["spec_versions"]],
        "expected_order_hash": split["order"]["order_hash"],
    }
    merge_preflight_response = await client.post(
        "/api/v1/shots/merge-preflight",
        headers=headers,
        json=merge_preflight_payload,
    )
    assert merge_preflight_response.status_code == 200
    merge_preflight = merge_preflight_response.json()["data"]
    assert merge_preflight["operation"] == "merge"
    merged_response = await client.post(
        "/api/v1/shots/merge",
        headers=headers,
        json={
            **merge_preflight_payload,
            "impact_hash": merge_preflight["impact_hash"],
            "idempotency_key": "merge-transform-001",
            "target": {
                "title": "进入并观察月台",
                "spec": shot_spec_payload(refs, purpose="合并后的完整叙事目标"),
                "asset_references": [],
            },
        },
    )
    assert merged_response.status_code == 201
    merged = merged_response.json()["data"]
    assert merged["transform"]["operation"] == "merge"
    assert len(merged["shots"]) == 1
    assert merged["shots"][0]["position"] == 1
    assert merged["spec_versions"][0]["spec"]["duration_ms"] == 3000
    assert [item["status"] for item in split["shots"]] == ["active", "active"]
    for split_shot in split["shots"]:
        persisted = await client.get(f"/api/v1/shots/{split_shot['id']}", headers=headers)
        assert persisted.json()["data"]["status"] == "archived"

    transform_audit = await client.get(
        "/api/v1/audit-events",
        headers=headers,
        params={
            "workspace_id": str(refs["workspace_id"]),
            "action": "shot.spec_version_created",
        },
    )
    assert transform_audit.status_code == 200
    assert transform_audit.json()["data"]["total"] == 5
    sources = [item["metadata"]["source"] for item in transform_audit.json()["data"]["items"]]
    assert sources.count("manual_save") == 1
    assert sources.count("copy") == 1
    assert sources.count("split") == 2
    assert sources.count("merge") == 1


@pytest.mark.asyncio
async def test_readiness_is_deterministic_and_reacts_to_rights_without_mutating_spec(
    client: httpx.AsyncClient,
    session_factory: async_sessionmaker[AsyncSession],
) -> None:
    headers, episode, refs = await create_episode_with_confirmed_structure(
        client,
        session_factory,
        email="storyboard-readiness@example.com",
    )
    created = await client.post(
        f"/api/v1/episodes/{episode['id']}/shots",
        headers=headers,
        json=shot_creation_payload(
            refs,
            title="准备度镜头",
            creation_key="readiness-shot-001",
        ),
    )
    assert created.status_code == 201
    shot = created.json()["data"]

    missing_response = await client.get(
        f"/api/v1/shots/{shot['id']}/readiness",
        headers=headers,
    )
    assert missing_response.status_code == 200
    missing = missing_response.json()["data"]
    assert missing["status"] == "blocked"
    assert missing["ready"] is False
    assert [item["code"] for item in missing["blocking_reasons"]] == ["CURRENT_SPEC_MISSING"]
    assert missing["next_actions"] == ["save_shot_spec"]
    assert len(missing["evaluation_hash"]) == 64

    location_version, consent = await create_ready_location_asset(
        client,
        session_factory,
        headers=headers,
        project_id=UUID(episode["project_id"]),
        refs=refs,
    )
    ready_spec = deepcopy(shot_spec_payload(refs, purpose="建立雨夜车站空间"))
    visual = dict(cast(dict[str, object], ready_spec["visual"]))
    visual["subject_placements"] = []
    ready_spec["visual"] = visual
    ready_spec["dialogue_or_narration"] = []
    saved_response = await client.post(
        f"/api/v1/shots/{shot['id']}/spec-versions",
        headers=headers,
        json={
            "expected_current_spec_version_id": None,
            "spec": ready_spec,
            "asset_references": [
                {
                    "slot_key": "location-main",
                    "role": "location",
                    "asset_version_id": location_version["id"],
                    "subject_key": None,
                }
            ],
        },
    )
    assert saved_response.status_code == 201
    spec_version = saved_response.json()["data"]["version"]

    first_response = await client.get(
        f"/api/v1/shots/{shot['id']}/readiness",
        headers=headers,
    )
    second_response = await client.get(
        f"/api/v1/shots/{shot['id']}/readiness",
        headers=headers,
    )
    assert first_response.status_code == second_response.status_code == 200
    first = first_response.json()["data"]
    second = second_response.json()["data"]
    assert first["status"] == "ready"
    assert first["ready"] is True
    assert first["blocking_reasons"] == []
    assert [item["code"] for item in first["warnings"]] == ["STYLE_REFERENCE_MISSING"]
    assert first["evaluation_hash"] == second["evaluation_hash"]
    assert first["evaluated_dependencies"]["shot_spec_version_id"] == spec_version["id"]
    assert first["evaluated_dependencies"]["asset_version_ids"] == [location_version["id"]]
    assert first["evaluated_dependencies"]["consent_ids"] == [consent["id"]]

    revoked = await client.post(
        f"/api/v1/consents/{consent['id']}/revoke",
        headers=headers,
        json={"expected_revision": 1, "reason": "撤销分镜生成授权"},
    )
    assert revoked.status_code == 200
    blocked_response = await client.get(
        f"/api/v1/shots/{shot['id']}/readiness",
        headers=headers,
    )
    assert blocked_response.status_code == 200
    blocked = blocked_response.json()["data"]
    assert blocked["status"] == "blocked"
    assert blocked["ready"] is False
    assert [item["code"] for item in blocked["blocking_reasons"]] == ["RIGHTS_BLOCKED"]
    assert blocked["evaluation_hash"] != first["evaluation_hash"]

    persisted_spec = await client.get(
        f"/api/v1/shot-spec-versions/{spec_version['id']}",
        headers=headers,
    )
    assert persisted_spec.status_code == 200
    assert persisted_spec.json()["data"] == spec_version


@pytest.mark.asyncio
@pytest.mark.parametrize("shot_count", [36, 120])
async def test_batch_readiness_has_constant_query_bound(
    client: httpx.AsyncClient,
    session_factory: async_sessionmaker[AsyncSession],
    shot_count: int,
) -> None:
    headers, episode, refs = await create_episode_with_confirmed_structure(
        client,
        session_factory,
        email=f"storyboard-batch-readiness-{shot_count}@example.com",
    )
    location_version, _ = await create_ready_location_asset(
        client,
        session_factory,
        headers=headers,
        project_id=UUID(episode["project_id"]),
        refs=refs,
    )
    await _seed_ready_storyboard_shots(
        session_factory,
        refs=refs,
        location_version_id=UUID(location_version["id"]),
        count=shot_count,
    )

    engine = session_factory.kw.get("bind")
    assert isinstance(engine, AsyncEngine)
    statements: list[str] = []

    def _count_statement(
        _connection: object,
        _cursor: object,
        statement: str,
        _parameters: object,
        _context: object,
        _executemany: object,
    ) -> None:
        statements.append(statement)

    event.listen(engine.sync_engine, "before_cursor_execute", _count_statement)
    try:
        response = await client.get(
            f"/api/v1/episodes/{episode['id']}/shot-readiness",
            headers=headers,
        )
    finally:
        event.remove(engine.sync_engine, "before_cursor_execute", _count_statement)

    assert response.status_code == 200
    result = response.json()["data"]
    assert len(result["items"]) == shot_count
    assert result["summary"] == {
        "total": shot_count,
        "ready": shot_count,
        "blocked": 0,
        "unavailable": 0,
    }
    assert all(item["status"] == "ready" for item in result["items"])
    assert len(result["evaluation_hash"]) == 64
    # Current narrative identity and dependency hash add two batch queries;
    # the bound must remain independent of shot count.
    assert len(statements) <= 14, [statement.splitlines()[0] for statement in statements]

    snapshot_statements: list[str] = []

    def _count_snapshot_statement(
        _connection: object,
        _cursor: object,
        statement: str,
        _parameters: object,
        _context: object,
        _executemany: object,
    ) -> None:
        snapshot_statements.append(statement)

    event.listen(
        engine.sync_engine,
        "before_cursor_execute",
        _count_snapshot_statement,
    )
    try:
        snapshot_response = await client.get(
            f"/api/v1/episodes/{episode['id']}/production-snapshot",
            headers=headers,
        )
    finally:
        event.remove(
            engine.sync_engine,
            "before_cursor_execute",
            _count_snapshot_statement,
        )

    assert snapshot_response.status_code == 200
    assert snapshot_response.json()["data"]["storyboard_summary"] == {
        "status": "ready",
        "total": shot_count,
        "ready": shot_count,
        "blocked": 0,
        "unavailable": 0,
    }
    assert len(snapshot_statements) <= 26, [
        statement.splitlines()[0] for statement in snapshot_statements
    ]


@pytest.mark.asyncio
async def test_asset_usage_and_upgrade_are_append_only_and_all_or_nothing(
    client: httpx.AsyncClient,
    session_factory: async_sessionmaker[AsyncSession],
) -> None:
    headers, episode, refs = await create_episode_with_confirmed_structure(
        client,
        session_factory,
        email="storyboard-asset-upgrade@example.com",
    )
    old_version, _ = await create_ready_location_asset(
        client,
        session_factory,
        headers=headers,
        project_id=UUID(episode["project_id"]),
        refs=refs,
    )
    new_version = await _append_ready_location_version(
        client,
        headers=headers,
        refs=refs,
        current_version=old_version,
    )
    ready_spec = deepcopy(shot_spec_payload(refs, purpose="资产升级前的固定镜头"))
    visual = dict(cast(dict[str, object], ready_spec["visual"]))
    visual["subject_placements"] = []
    ready_spec["visual"] = visual
    ready_spec["dialogue_or_narration"] = []

    shots: list[dict[str, Any]] = []
    saved_versions: list[dict[str, Any]] = []
    for index in range(2):
        created = await client.post(
            f"/api/v1/episodes/{episode['id']}/shots",
            headers=headers,
            json=shot_creation_payload(
                refs,
                title=f"资产升级镜头 {index + 1}",
                creation_key=f"asset-upgrade-shot-{index + 1}",
            ),
        )
        assert created.status_code == 201
        shot = created.json()["data"]
        saved = await client.post(
            f"/api/v1/shots/{shot['id']}/spec-versions",
            headers=headers,
            json={
                "expected_current_spec_version_id": None,
                "spec": ready_spec,
                "asset_references": [
                    {
                        "slot_key": "location-main",
                        "role": "location",
                        "asset_version_id": old_version["id"],
                        "subject_key": None,
                    }
                ],
            },
        )
        assert saved.status_code == 201
        shots.append(saved.json()["data"]["shot"])
        saved_versions.append(saved.json()["data"]["version"])

    usage_response = await client.get(
        f"/api/v1/asset-versions/{old_version['id']}/shot-usages",
        headers=headers,
    )
    assert usage_response.status_code == 200
    usage = usage_response.json()["data"]
    assert usage["total"] == 2
    assert all(item["is_current"] for item in usage["items"])
    assert {item["shot_id"] for item in usage["items"]} == {shot["id"] for shot in shots}

    preflight_payload = {
        "new_asset_version_id": new_version["id"],
        "shot_ids": [shot["id"] for shot in shots],
    }
    preflight_response = await client.post(
        f"/api/v1/asset-versions/{old_version['id']}/upgrade-preflight",
        headers=headers,
        json=preflight_payload,
    )
    assert preflight_response.status_code == 200
    preflight = preflight_response.json()["data"]
    assert len(preflight["targets"]) == 2
    assert all(len(target["new_input_hash"]) == 64 for target in preflight["targets"])
    assert len(preflight["preflight_hash"]) == 64

    changed_spec = deepcopy(ready_spec)
    changed_spec["narrative"] = {
        "purpose": "并发修改第二个镜头但仍引用旧资产",
        "continuity_note": None,
    }
    concurrent_change = await client.post(
        f"/api/v1/shots/{shots[1]['id']}/spec-versions",
        headers=headers,
        json={
            "expected_current_spec_version_id": saved_versions[1]["id"],
            "spec": changed_spec,
            "asset_references": [
                {
                    "slot_key": "location-main",
                    "role": "location",
                    "asset_version_id": old_version["id"],
                    "subject_key": None,
                }
            ],
        },
    )
    assert concurrent_change.status_code == 201

    stale_apply = await client.post(
        f"/api/v1/asset-versions/{old_version['id']}/upgrade",
        headers=headers,
        json={
            "new_asset_version_id": new_version["id"],
            "targets": preflight["targets"],
            "preflight_hash": preflight["preflight_hash"],
        },
    )
    assert stale_apply.status_code == 409
    assert stale_apply.json()["error"]["code"] == "version_conflict"
    first_history = await client.get(
        f"/api/v1/shots/{shots[0]['id']}/spec-versions",
        headers=headers,
    )
    assert len(first_history.json()["data"]) == 1
    assert (
        first_history.json()["data"][0]["asset_references"][0]["asset_version_id"]
        == old_version["id"]
    )

    fresh_preflight_response = await client.post(
        f"/api/v1/asset-versions/{old_version['id']}/upgrade-preflight",
        headers=headers,
        json=preflight_payload,
    )
    assert fresh_preflight_response.status_code == 200
    fresh = fresh_preflight_response.json()["data"]
    applied_response = await client.post(
        f"/api/v1/asset-versions/{old_version['id']}/upgrade",
        headers=headers,
        json={
            "new_asset_version_id": new_version["id"],
            "targets": fresh["targets"],
            "preflight_hash": fresh["preflight_hash"],
        },
    )
    assert applied_response.status_code == 201
    applied = applied_response.json()["data"]
    assert len(applied["spec_versions"]) == 2
    assert all(
        version["asset_references"][0]["asset_version_id"] == new_version["id"]
        for version in applied["spec_versions"]
    )

    old_spec = await client.get(
        f"/api/v1/shot-spec-versions/{saved_versions[0]['id']}",
        headers=headers,
    )
    assert old_spec.status_code == 200
    assert old_spec.json()["data"] == saved_versions[0]
    old_usage = await client.get(
        f"/api/v1/asset-versions/{old_version['id']}/shot-usages",
        headers=headers,
    )
    new_usage = await client.get(
        f"/api/v1/asset-versions/{new_version['id']}/shot-usages",
        headers=headers,
    )
    assert old_usage.json()["data"]["total"] == 3
    assert all(not item["is_current"] for item in old_usage.json()["data"]["items"])
    assert new_usage.json()["data"]["total"] == 2

    upgrade_audit = await client.get(
        "/api/v1/audit-events",
        headers=headers,
        params={
            "workspace_id": str(refs["workspace_id"]),
            "action": "shot.spec_version_created",
        },
    )
    assert upgrade_audit.status_code == 200
    assert upgrade_audit.json()["data"]["total"] == 5
    upgrade_sources = [item["metadata"]["source"] for item in upgrade_audit.json()["data"]["items"]]
    assert upgrade_sources.count("manual_save") == 3
    assert upgrade_sources.count("asset_upgrade") == 2
    assert all(item["is_current"] for item in new_usage.json()["data"]["items"])

    shot_order = await client.get(
        f"/api/v1/episodes/{episode['id']}/shots",
        headers=headers,
    )
    assert shot_order.status_code == 200
    archived = await client.post(
        f"/api/v1/shots/{shots[0]['id']}/archive",
        headers=headers,
        json={
            "expected_revision": applied["shots"][0]["revision"],
            "expected_order_hash": shot_order.json()["data"]["order_hash"],
        },
    )
    assert archived.status_code == 200
    usage_after_archive = await client.get(
        f"/api/v1/asset-versions/{new_version['id']}/shot-usages",
        headers=headers,
    )
    assert usage_after_archive.status_code == 200
    usage_by_shot_id = {
        item["shot_id"]: item for item in usage_after_archive.json()["data"]["items"]
    }
    assert usage_by_shot_id[shots[0]["id"]]["is_current"] is False
    assert usage_by_shot_id[shots[1]["id"]]["is_current"] is True


@pytest.mark.asyncio
async def test_production_snapshot_is_stable_scoped_and_provider_agnostic(
    client: httpx.AsyncClient,
    session_factory: async_sessionmaker[AsyncSession],
) -> None:
    headers, episode, refs = await create_episode_with_confirmed_structure(
        client,
        session_factory,
        email="storyboard-production-snapshot@example.com",
    )
    location_version, consent = await create_ready_location_asset(
        client,
        session_factory,
        headers=headers,
        project_id=UUID(episode["project_id"]),
        refs=refs,
    )
    created = await client.post(
        f"/api/v1/episodes/{episode['id']}/shots",
        headers=headers,
        json=shot_creation_payload(
            refs,
            title="生产快照镜头",
            creation_key="production-snapshot-shot",
        ),
    )
    assert created.status_code == 201
    shot = created.json()["data"]
    spec_payload = deepcopy(shot_spec_payload(refs, purpose="形成稳定生产输入"))
    visual = dict(cast(dict[str, object], spec_payload["visual"]))
    visual["subject_placements"] = []
    spec_payload["visual"] = visual
    spec_payload["dialogue_or_narration"] = []
    saved = await client.post(
        f"/api/v1/shots/{shot['id']}/spec-versions",
        headers=headers,
        json={
            "expected_current_spec_version_id": None,
            "spec": spec_payload,
            "asset_references": [
                {
                    "slot_key": "location-main",
                    "role": "location",
                    "asset_version_id": location_version["id"],
                    "subject_key": None,
                }
            ],
        },
    )
    assert saved.status_code == 201
    version = saved.json()["data"]["version"]

    from app.modules import storyboards

    snapshot_reader = cast(
        _SnapshotReader | None,
        getattr(storyboards, "get_production_snapshot", None),
    )
    assert snapshot_reader is not None
    async with session_factory() as session:
        snapshot = await snapshot_reader(
            session,
            refs["workspace_id"],
            UUID(version["id"]),
        )
        hidden = await snapshot_reader(
            session,
            uuid7(),
            UUID(version["id"]),
        )
    assert hidden is None
    assert snapshot is not None
    assert snapshot.spec_ref.shot_id == UUID(shot["id"])
    assert snapshot.spec_ref.shot_spec_version_id == UUID(version["id"])
    assert snapshot.spec_ref.input_hash == version["input_hash"]
    assert snapshot.readiness_status == "ready"
    assert snapshot.ready is True
    assert snapshot.asset_references[0].asset_version_id == UUID(location_version["id"])
    assert snapshot.spec["generation_intent"]["mode"] == "text_to_video"
    serialized = str(snapshot).lower()
    assert "provider" not in serialized
    assert "model_id" not in serialized
    assert "cost" not in serialized

    revoked = await client.post(
        f"/api/v1/consents/{consent['id']}/revoke",
        headers=headers,
        json={"expected_revision": 1, "reason": "撤销生产快照授权"},
    )
    assert revoked.status_code == 200
    async with session_factory() as session:
        blocked = await snapshot_reader(
            session,
            refs["workspace_id"],
            UUID(version["id"]),
        )
    assert blocked is not None
    assert blocked.readiness_status == "blocked"
    assert blocked.ready is False
    assert blocked.spec_ref == snapshot.spec_ref
    assert blocked.evaluation_hash != snapshot.evaluation_hash
