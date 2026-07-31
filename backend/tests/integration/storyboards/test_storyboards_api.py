from copy import deepcopy
from typing import Any, cast
from uuid import UUID

import httpx
import pytest
from sqlalchemy import select
from sqlalchemy.ext.asyncio import AsyncSession, async_sessionmaker
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


@pytest.mark.asyncio
async def test_confirmed_candidate_creation_and_safe_delete_preserve_evidence(
    client: httpx.AsyncClient,
    session_factory: async_sessionmaker[AsyncSession],
) -> None:
    headers, episode, refs = await _episode_with_confirmed_structure(
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
    assert [
        blocker["code"] for blocker in candidate_preflight.json()["data"]["blockers"]
    ] == ["SOURCE_CANDIDATE_EVIDENCE"]

    endpoint = f"/api/v1/episodes/{episode['id']}/shots"
    empty_response = await client.post(
        endpoint,
        headers=headers,
        json=_create_shot_payload(
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
    spec = deepcopy(_spec_payload(refs, purpose=purpose))
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
    headers, episode, refs = await _episode_with_confirmed_structure(
        client,
        session_factory,
        email="storyboard-transform@example.com",
    )
    endpoint = f"/api/v1/episodes/{episode['id']}/shots"
    created = await client.post(
        endpoint,
        headers=headers,
        json=_create_shot_payload(
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
            "spec": _spec_payload(refs, purpose="建立车站悬疑氛围"),
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
    assert copied["spec_versions"][0]["content_hash"] == source_spec[
        "content_hash"
    ]
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
    archived_source = await client.get(
        f"/api/v1/shots/{source['id']}", headers=headers
    )
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
        "expected_spec_version_ids": [
            version["id"] for version in split["spec_versions"]
        ],
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
                "spec": _spec_payload(refs, purpose="合并后的完整叙事目标"),
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
        persisted = await client.get(
            f"/api/v1/shots/{split_shot['id']}", headers=headers
        )
        assert persisted.json()["data"]["status"] == "archived"
