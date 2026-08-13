from datetime import UTC, datetime, timedelta
from typing import Any
from uuid import UUID

import httpx
import pytest
from sqlalchemy import func, select, text
from sqlalchemy.ext.asyncio import AsyncSession, async_sessionmaker
from uuid6 import uuid7

from app.modules.assets.models import AssetVersion
from app.modules.production.models import (
    GenerationRequest,
    GenerationRequestAsset,
    ModelCapability,
    Task,
)
from app.modules.projects.models import Episode
from app.modules.scripts.models import Scene, ScriptSource, ScriptVersion
from app.modules.storyboards.models import AssetReference, Shot, ShotSpecVersion
from tests.support.identity_builders import register_identity_response
from tests.support.project_builders import project_payload


async def _asset_change_facts(
    client: httpx.AsyncClient,
    session_factory: async_sessionmaker[AsyncSession],
    *,
    email: str,
) -> tuple[dict[str, str], dict[str, Any], dict[str, UUID]]:
    registered = await register_identity_response(client, email=email)
    assert registered.status_code == 201
    identity = registered.json()["data"]
    headers = {"authorization": f"Bearer {identity['access_token']}"}
    project_response = await client.post(
        "/api/v1/projects",
        headers=headers,
        json=project_payload(identity["workspace"]["id"], "资产影响验收项目"),
    )
    assert project_response.status_code == 201
    project = project_response.json()["data"]
    episode_response = await client.post(
        f"/api/v1/projects/{project['id']}/episodes",
        headers=headers,
        json={"name": "第一集", "target_duration_ms": 90_000},
    )
    assert episode_response.status_code == 201
    episode = episode_response.json()["data"]
    asset_response = await client.post(
        f"/api/v1/projects/{project['id']}/assets",
        headers=headers,
        json={
            "kind": "location",
            "name": "旧泵站",
            "aliases": ["泵站"],
            "tags": ["主场景"],
        },
    )
    assert asset_response.status_code == 201
    asset = asset_response.json()["data"]
    states_response = await client.get(
        f"/api/v1/assets/{asset['id']}/states",
        headers=headers,
    )
    state = states_response.json()["data"]["items"][0]
    first_version_response = await client.post(
        f"/api/v1/asset-states/{state['id']}/versions",
        headers=headers,
        json={
            "spec": {
                "kind": "location",
                "spatial_description": "封闭泵房与生锈闸门",
                "time_weather": "暴雨夜",
                "visual_elements": ["控制台", "积水"],
                "lighting": "冷蓝应急灯",
            },
            "prompt_description": "固定旧泵站空间",
            "media_references": [],
            "source_type": "manual",
            "source_id": None,
            "expected_revision": state["revision"],
            "expected_current_version_id": None,
            "set_as_current": True,
        },
    )
    assert first_version_response.status_code == 201
    first_data = first_version_response.json()["data"]
    state = first_data["state"]
    first_version = first_data["version"]
    second_version_response = await client.post(
        f"/api/v1/asset-states/{state['id']}/versions",
        headers=headers,
        json={
            "spec": {
                "kind": "location",
                "spatial_description": "洪水漫入泵房后的空间",
                "time_weather": "暴雨夜",
                "visual_elements": ["破损控制台", "深水"],
                "lighting": "红色警报灯",
            },
            "prompt_description": "固定被淹后的泵站空间",
            "media_references": [],
            "source_type": "manual",
            "source_id": None,
            "expected_revision": state["revision"],
            "expected_current_version_id": first_version["id"],
            "set_as_current": False,
        },
    )
    assert second_version_response.status_code == 201
    second_data = second_version_response.json()["data"]

    workspace_id = UUID(identity["workspace"]["id"])
    actor_id = UUID(identity["user"]["id"])
    project_id = UUID(project["id"])
    episode_id = UUID(episode["id"])
    asset_id = UUID(asset["id"])
    state_id = UUID(state["id"])
    first_version_id = UUID(first_version["id"])
    source_id = uuid7()
    script_version_id = uuid7()
    scene_id = uuid7()
    shot_id = uuid7()
    spec_version_id = uuid7()
    request_id = uuid7()
    task_id = uuid7()
    now = datetime.now(UTC)
    async with session_factory() as session, session.begin():
        source = ScriptSource(
            id=source_id,
            workspace_id=workspace_id,
            episode_id=episode_id,
            input_type="text",
            title="资产影响测试稿",
            rights_declaration="原创自动化测试文本",
            status="active",
            revision=1,
            idempotency_key=f"asset-impact:{episode_id}",
        )
        script_version = ScriptVersion(
            id=script_version_id,
            workspace_id=workspace_id,
            source_id=source_id,
            version_no=1,
            status="published",
            body="内景·旧泵站·夜\n沈岚冲向控制台。",
            content_hash="a" * 64,
            structure_summary={
                "confirmation_batch_id": str(uuid7()),
                "source_script_version_id": str(uuid7()),
                "scene_count": 1,
                "dialogue_count": 0,
            },
            created_by=actor_id,
        )
        scene = Scene(
            id=scene_id,
            workspace_id=workspace_id,
            script_version_id=script_version_id,
            position=1,
            heading="内景·旧泵站·夜",
            location="旧泵站",
            time_of_day="夜",
            summary="沈岚冲向控制台",
            source_start=0,
            source_end=18,
        )
        shot = Shot(
            id=shot_id,
            workspace_id=workspace_id,
            episode_id=episode_id,
            position=1,
            title="冲向控制台",
            source_script_version_id=script_version_id,
            source_scene_id=scene_id,
            creation_key=f"asset-impact-shot:{episode_id}",
            status="active",
            current_spec_version_id=spec_version_id,
            revision=2,
            created_by=actor_id,
        )
        spec = ShotSpecVersion(
            id=spec_version_id,
            workspace_id=workspace_id,
            shot_id=shot_id,
            version_no=1,
            schema_version=1,
            spec={
                "schema_version": 1,
                "script_reference": {
                    "confirmed_script_version_id": str(script_version_id),
                    "scene_id": str(scene_id),
                    "dialogue_ids": [],
                },
                "narrative": {"purpose": "建立危机", "continuity_note": None},
                "visual": {
                    "shot_size": "wide",
                    "camera_angle": "eye_level",
                    "camera_movement": "static",
                    "composition": "人物冲向控制台",
                    "environment": "旧泵站",
                    "subject_placements": [],
                    "mood_lighting": "冷蓝应急灯",
                },
                "action_beats": [],
                "dialogue_or_narration": [],
                "duration_ms": 4000,
                "audio_intent": {"ambient": "水声", "sound_effects": []},
                "generation_intent": {
                    "mode": "text_to_video",
                    "first_frame": None,
                    "last_frame": None,
                    "keyframe_notes": None,
                },
            },
            content_hash="b" * 64,
            input_hash="c" * 64,
            created_by=actor_id,
        )
        asset_reference = AssetReference(
            id=uuid7(),
            workspace_id=workspace_id,
            shot_spec_version_id=spec_version_id,
            slot_key="location-main",
            role="location",
            asset_version_id=first_version_id,
            asset_state_id=state_id,
            asset_id=asset_id,
            binding_source="manual",
            subject_key=None,
        )
        capability = ModelCapability(
            id=uuid7(),
            provider="test",
            model="asset-impact-model",
            kind="image",
            config_version=1,
            input_types=["text"],
            parameter_schema={"type": "object", "additionalProperties": False},
            limits={},
            pricing={"currency": "CNY", "fixed_amount": "1.000000"},
            status="active",
            created_at=now,
        )
        generation_request = GenerationRequest(
            id=request_id,
            workspace_id=workspace_id,
            project_id=project_id,
            episode_id=episode_id,
            shot_id=shot_id,
            shot_spec_version_id=spec_version_id,
            capability_id=capability.id,
            capability_config_version=1,
            parameter_snapshot={},
            warning_acknowledgements=[],
            shot_spec_input_hash=spec.input_hash,
            input_hash="d" * 64,
            preflight_hash="e" * 64,
            preflight_expires_at=now + timedelta(minutes=10),
            high_cost_confirmed=False,
            idempotency_key=f"asset-impact-generation:{episode_id}",
            requested_by=actor_id,
            created_at=now,
        )
        request_asset = GenerationRequestAsset(
            id=uuid7(),
            workspace_id=workspace_id,
            request_id=request_id,
            asset_version_id=first_version_id,
            slot_key="location-main",
            created_at=now,
        )
        task = Task(
            id=task_id,
            workspace_id=workspace_id,
            task_type="image_generation",
            request_type="generation_request",
            request_id=request_id,
            episode_id=episode_id,
            usage_type="shot",
            usage_id=shot_id,
            input_version_id=spec_version_id,
            input_hash=generation_request.input_hash,
            status="queued",
            progress_stage="queued",
            next_action="poll_task",
            cancel_status="none",
            idempotency_key=f"asset-impact-task:{episode_id}",
            requested_by=actor_id,
            revision=1,
            created_at=now,
            updated_at=now,
        )
        session.add_all([source, capability])
        await session.flush()
        session.add(script_version)
        await session.flush()
        session.add(scene)
        persisted_episode = await session.scalar(
            select(Episode).where(Episode.id == episode_id).with_for_update()
        )
        assert persisted_episode is not None
        persisted_episode.current_script_version_id = script_version_id
        persisted_episode.revision += 1
        session.add(shot)
        await session.flush()
        session.add(spec)
        await session.flush()
        session.add(generation_request)
        await session.flush()
        session.add_all([asset_reference, request_asset, task])

    return headers, asset, {
        "workspace_id": workspace_id,
        "project_id": project_id,
        "episode_id": episode_id,
        "asset_id": asset_id,
        "state_id": state_id,
        "first_version_id": first_version_id,
        "second_version_id": UUID(second_data["version"]["id"]),
        "shot_id": shot_id,
        "spec_version_id": spec_version_id,
        "request_id": request_id,
        "task_id": task_id,
    }


@pytest.mark.asyncio
async def test_rename_uses_stable_impact_and_keeps_all_references(
    client: httpx.AsyncClient,
    session_factory: async_sessionmaker[AsyncSession],
) -> None:
    headers, asset, facts = await _asset_change_facts(
        client,
        session_factory,
        email="asset-rename-impact@example.com",
    )
    preflight_request = {
        "new_name": "北区排涝站",
        "expected_revision": asset["revision"],
    }
    preflight_response = await client.post(
        f"/api/v1/assets/{asset['id']}/rename-preflight",
        headers=headers,
        json=preflight_request,
    )
    assert preflight_response.status_code == 200
    preflight = preflight_response.json()["data"]
    assert preflight["operation"] == "rename"
    assert len(preflight["impact_hash"]) == 64
    assert preflight["summary"] == {
        "episode_count": 1,
        "shot_count": 1,
        "spec_version_count": 1,
        "prompt_snapshot_count": 1,
        "active_task_count": 1,
    }
    assert preflight["shots"][0]["slot_keys"] == ["location-main"]
    assert preflight["prompt_snapshots"][0]["generation_request_id"] == str(
        facts["request_id"]
    )
    assert preflight["active_tasks"][0]["task_id"] == str(facts["task_id"])

    async with session_factory() as session, session.begin():
        task = await session.scalar(
            select(Task).where(Task.id == facts["task_id"]).with_for_update()
        )
        assert task is not None
        task.status = "running"
        task.progress_stage = "provider_submit"
        task.revision += 1

    stale_response = await client.post(
        f"/api/v1/assets/{asset['id']}/rename",
        headers=headers,
        json={
            **preflight_request,
            "impact_hash": preflight["impact_hash"],
            "idempotency_key": "rename:pump-station:v1",
        },
    )
    assert stale_response.status_code == 409
    assert stale_response.json()["error"]["code"] == "version_conflict"
    unchanged = await client.get(f"/api/v1/assets/{asset['id']}", headers=headers)
    assert unchanged.json()["data"]["name"] == "旧泵站"

    fresh_response = await client.post(
        f"/api/v1/assets/{asset['id']}/rename-preflight",
        headers=headers,
        json=preflight_request,
    )
    fresh = fresh_response.json()["data"]
    renamed_response = await client.post(
        f"/api/v1/assets/{asset['id']}/rename",
        headers=headers,
        json={
            **preflight_request,
            "impact_hash": fresh["impact_hash"],
            "idempotency_key": "rename:pump-station:v2",
        },
    )
    assert renamed_response.status_code == 200
    renamed = renamed_response.json()["data"]["asset"]
    assert renamed["id"] == asset["id"]
    assert renamed["name"] == "北区排涝站"
    assert renamed["name_revision"] == 2
    assert renamed["aliases"] == ["泵站", "旧泵站"]

    async with session_factory() as session:
        reference = await session.scalar(
            select(AssetReference).where(
                AssetReference.shot_spec_version_id == facts["spec_version_id"]
            )
        )
        assert reference is not None
        assert reference.asset_id == facts["asset_id"]
        assert reference.asset_state_id == facts["state_id"]
        assert reference.asset_version_id == facts["first_version_id"]
        names = (
            await session.execute(
                text(
                    "SELECT revision_no, name_snapshot FROM ast_asset_name_revisions "
                    "WHERE asset_id = :asset_id ORDER BY revision_no"
                ),
                {"asset_id": facts["asset_id"]},
            )
        ).all()
    assert names == [(1, "旧泵站"), (2, "北区排涝站")]


@pytest.mark.asyncio
async def test_disable_blocks_new_work_without_deleting_history(
    client: httpx.AsyncClient,
    session_factory: async_sessionmaker[AsyncSession],
) -> None:
    headers, asset, facts = await _asset_change_facts(
        client,
        session_factory,
        email="asset-disable-impact@example.com",
    )
    preflight_response = await client.post(
        f"/api/v1/assets/{asset['id']}/disable-preflight",
        headers=headers,
        json={"expected_revision": asset["revision"]},
    )
    assert preflight_response.status_code == 200
    impact = preflight_response.json()["data"]
    disabled_response = await client.post(
        f"/api/v1/assets/{asset['id']}/disable",
        headers=headers,
        json={
            "expected_revision": asset["revision"],
            "impact_hash": impact["impact_hash"],
            "idempotency_key": "disable:pump-station:v1",
        },
    )
    assert disabled_response.status_code == 200
    disabled = disabled_response.json()["data"]["asset"]
    assert disabled["availability"] == "disabled"

    readiness_response = await client.get(
        f"/api/v1/shots/{facts['shot_id']}/readiness",
        headers=headers,
    )
    assert readiness_response.status_code == 200
    readiness = readiness_response.json()["data"]
    assert readiness["ready"] is False
    assert "asset_disabled" in {
        blocker["code"] for blocker in readiness["blocking_reasons"]
    }

    async with session_factory() as session:
        assert await session.scalar(
            select(func.count()).select_from(AssetVersion).where(
                AssetVersion.asset_id == facts["asset_id"]
            )
        ) == 2
        assert await session.scalar(
            select(func.count()).select_from(AssetReference).where(
                AssetReference.asset_id == facts["asset_id"]
            )
        ) == 1
        assert await session.scalar(
            select(func.count()).select_from(GenerationRequestAsset).where(
                GenerationRequestAsset.asset_version_id == facts["first_version_id"]
            )
        ) == 1
        assert await session.scalar(
            select(func.count()).select_from(Task).where(Task.id == facts["task_id"])
        ) == 1

    enabled_response = await client.post(
        f"/api/v1/assets/{asset['id']}/enable",
        headers=headers,
        json={
            "expected_revision": disabled["revision"],
            "idempotency_key": "enable:pump-station:v1",
        },
    )
    assert enabled_response.status_code == 200
    assert enabled_response.json()["data"]["availability"] == "enabled"


@pytest.mark.asyncio
async def test_state_edit_disable_and_current_switch_require_explicit_impacts(
    client: httpx.AsyncClient,
    session_factory: async_sessionmaker[AsyncSession],
) -> None:
    headers, _, facts = await _asset_change_facts(
        client,
        session_factory,
        email="asset-state-change@example.com",
    )
    state_response = await client.get(
        f"/api/v1/assets/{facts['asset_id']}/states",
        headers=headers,
    )
    state = state_response.json()["data"]["items"][0]
    edited_response = await client.patch(
        f"/api/v1/asset-states/{state['id']}",
        headers=headers,
        json={
            "expected_revision": state["revision"],
            "label": "暴雨夜",
            "description": "泵站仍可运行但水位持续上涨",
        },
    )
    assert edited_response.status_code == 200
    edited = edited_response.json()["data"]

    current_preflight_response = await client.post(
        f"/api/v1/asset-states/{state['id']}/current-version-preflight",
        headers=headers,
        json={
            "version_id": str(facts["second_version_id"]),
            "expected_current_version_id": str(facts["first_version_id"]),
            "expected_revision": edited["revision"],
        },
    )
    assert current_preflight_response.status_code == 200
    current_impact = current_preflight_response.json()["data"]
    assert current_impact["summary"]["shot_count"] == 1
    current_response = await client.post(
        f"/api/v1/asset-states/{state['id']}/current-version",
        headers=headers,
        json={
            "version_id": str(facts["second_version_id"]),
            "expected_current_version_id": str(facts["first_version_id"]),
            "expected_revision": edited["revision"],
            "impact_hash": current_impact["impact_hash"],
            "idempotency_key": "state-current:flooded:v1",
        },
    )
    assert current_response.status_code == 200
    current = current_response.json()["data"]["state"]
    assert current["current_version_id"] == str(facts["second_version_id"])

    async with session_factory() as session:
        reference = await session.scalar(
            select(AssetReference).where(
                AssetReference.shot_spec_version_id == facts["spec_version_id"]
            )
        )
    assert reference is not None
    assert reference.asset_version_id == facts["first_version_id"]

    disable_preflight_response = await client.post(
        f"/api/v1/asset-states/{state['id']}/disable-preflight",
        headers=headers,
        json={"expected_revision": current["revision"]},
    )
    assert disable_preflight_response.status_code == 200
    disable_impact = disable_preflight_response.json()["data"]
    disable_response = await client.post(
        f"/api/v1/asset-states/{state['id']}/disable",
        headers=headers,
        json={
            "expected_revision": current["revision"],
            "impact_hash": disable_impact["impact_hash"],
            "idempotency_key": "state-disable:flooded:v1",
        },
    )
    assert disable_response.status_code == 200
    assert disable_response.json()["data"]["state"]["status"] == "disabled"

    enabled_response = await client.post(
        f"/api/v1/asset-states/{state['id']}/enable",
        headers=headers,
        json={
            "expected_revision": disable_response.json()["data"]["state"]["revision"],
            "idempotency_key": "state-enable:flooded:v1",
        },
    )
    assert enabled_response.status_code == 200
    assert enabled_response.json()["data"]["status"] == "active"
