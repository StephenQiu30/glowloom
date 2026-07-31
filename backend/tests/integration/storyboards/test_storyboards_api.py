from typing import Any
from uuid import UUID

import httpx
import pytest
from sqlalchemy import select
from sqlalchemy.ext.asyncio import AsyncSession, async_sessionmaker
from uuid6 import uuid7

from app.modules.projects.models import Episode
from app.modules.scripts.models import Dialogue, Scene, ScriptSource, ScriptVersion
from tests.support.identity_builders import register_identity_response
from tests.support.project_builders import project_payload


async def _episode_with_confirmed_structure(
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

    return headers, episode, {
        "workspace_id": workspace_id,
        "actor_id": actor_id,
        "episode_id": episode_id,
        "script_version_id": script_version_id,
        "scene_id": scene_id,
        "dialogue_id": dialogue_id,
    }


def _create_shot_payload(
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


def _spec_payload(refs: dict[str, UUID], *, purpose: str) -> dict[str, object]:
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
            "subject_placements": [
                {"subject_key": "hero", "placement": "画面中心"}
            ],
            "mood_lighting": "冷蓝顶光",
        },
        "action_beats": [
            {"beat_key": "pause", "order": 1, "description": "林澈停下脚步"}
        ],
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


@pytest.mark.asyncio
async def test_manual_shot_is_idempotent_ordered_and_lifecycle_safe(
    client: httpx.AsyncClient,
    session_factory: async_sessionmaker[AsyncSession],
) -> None:
    headers, episode, refs = await _episode_with_confirmed_structure(
        client,
        session_factory,
        email="storyboard-lifecycle@example.com",
    )
    endpoint = f"/api/v1/episodes/{episode['id']}/shots"
    first_payload = _create_shot_payload(
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
        json=_create_shot_payload(
            refs,
            title="发现灯箱",
            creation_key="manual-shot-002",
        ),
    )
    assert second_response.status_code == 201
    second = second_response.json()["data"]
    assert second["position"] == 2

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
    assert stale.json()["error"]["details"]["current_order_hash"] == new_order[
        "order_hash"
    ]

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
    assert [item["id"] for item in archived.json()["data"]["order"]["items"]] == [
        second["id"]
    ]

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


@pytest.mark.asyncio
async def test_shot_spec_versions_are_immutable_and_compare_current_pointer(
    client: httpx.AsyncClient,
    session_factory: async_sessionmaker[AsyncSession],
) -> None:
    headers, episode, refs = await _episode_with_confirmed_structure(
        client,
        session_factory,
        email="storyboard-spec@example.com",
    )
    created = await client.post(
        f"/api/v1/episodes/{episode['id']}/shots",
        headers=headers,
        json=_create_shot_payload(
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
            "spec": _spec_payload(refs, purpose="交代主角停下观察"),
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
            "spec": _spec_payload(refs, purpose="不能覆盖并发版本"),
            "asset_references": [],
        },
    )
    assert conflict.status_code == 409
    assert conflict.json()["error"]["code"] == "version_conflict"
    assert conflict.json()["error"]["details"]["current_spec_version_id"] == first[
        "version"
    ]["id"]

    second_response = await client.post(
        versions_endpoint,
        headers=headers,
        json={
            "expected_current_spec_version_id": first["version"]["id"],
            "spec": _spec_payload(refs, purpose="强化主角的警觉反应"),
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
    assert switched.json()["data"]["current_spec_version_id"] == first["version"][
        "id"
    ]

    invalid_dialogue = _spec_payload(refs, purpose="引用不属于场景的对白")
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
