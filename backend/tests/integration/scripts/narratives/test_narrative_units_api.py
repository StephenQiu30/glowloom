import asyncio
import json
from copy import deepcopy
from pathlib import Path
from typing import Any
from uuid import UUID

import httpx
import pytest
from sqlalchemy.ext.asyncio import AsyncSession, async_sessionmaker
from uuid6 import uuid7

from app.modules.scripts.models import Scene, ScriptVersion
from app.modules.storyboards.hashing import storyboard_content_hashes
from app.modules.storyboards.models import Shot, ShotSpecVersion
from app.modules.storyboards.schemas import ShotSpec
from tests.support.identity_builders import register_identity_response
from tests.support.project_builders import project_payload

GOLDEN_PATH = (
    Path(__file__).resolve().parents[3]
    / "fixtures"
    / "mvp_a"
    / "golden_candidate_harbor_countdown.json"
)
GOLDEN = json.loads(GOLDEN_PATH.read_text(encoding="utf-8"))
EPISODE_THREE = next(episode for episode in GOLDEN["episodes"] if episode["episode_id"] == "ep-03")
GOLDEN_UNITS = GOLDEN["narrative_units"]


async def _identity(
    client: httpx.AsyncClient,
    *,
    email: str,
) -> tuple[dict[str, str], dict[str, Any]]:
    response = await register_identity_response(
        client,
        email=email,
        password="a-secure-narrative-password",
        display_name="叙事结构负责人",
    )
    assert response.status_code == 201
    data = response.json()["data"]
    return {"authorization": f"Bearer {data['access_token']}"}, data


async def _episode(
    client: httpx.AsyncClient,
    headers: dict[str, str],
    identity: dict[str, Any],
    *,
    name: str,
) -> dict[str, Any]:
    project_response = await client.post(
        "/api/v1/projects",
        headers=headers,
        json=project_payload(identity["workspace"]["id"], f"{name}项目"),
    )
    assert project_response.status_code == 201
    project = project_response.json()["data"]
    episode_response = await client.post(
        f"/api/v1/projects/{project['id']}/episodes",
        headers=headers,
        json={"name": name, "target_duration_ms": 92_000},
    )
    assert episode_response.status_code == 201
    return episode_response.json()["data"]


async def _published_script(
    client: httpx.AsyncClient,
    *,
    email: str,
    body: str,
) -> tuple[dict[str, str], dict[str, Any], dict[str, Any], dict[str, Any]]:
    headers, identity = await _identity(client, email=email)
    episode = await _episode(client, headers, identity, name="第三集")
    imported_response = await client.post(
        f"/api/v1/episodes/{episode['id']}/script-sources",
        headers=headers,
        json={
            "input_type": "text",
            "title": "稳定叙事单元验收稿",
            "body": body,
            "rights_declaration": "原创测试文本，仅用于本地工程验收",
            "idempotency_key": f"narrative-source:{email}",
        },
    )
    assert imported_response.status_code == 201
    source = imported_response.json()["data"]["source"]
    published_response = await client.post(
        f"/api/v1/script-sources/{source['id']}/versions",
        headers=headers,
        json={"body": body, "expected_current_version_id": None},
    )
    assert published_response.status_code == 201
    version = published_response.json()["data"]["version"]
    return headers, episode, source, version


def _revision_payload(
    structure: dict[str, Any],
    *,
    idempotency_key: str,
) -> dict[str, Any]:
    return {
        "expected_revision": structure["revision"],
        "expected_current_script_version_id": structure["script_version_id"],
        "idempotency_key": idempotency_key,
        "units": [
            {
                "unit_id": unit["unit_id"],
                "kind": unit["kind"],
                "source_start": unit["source_range"]["start"],
                "source_end": unit["source_range"]["end"],
                "required_for_coverage": unit["required_for_coverage"],
            }
            for unit in structure["units"]
        ],
    }


@pytest.mark.asyncio
async def test_golden_units_use_stable_ids_and_append_human_corrections(
    client: httpx.AsyncClient,
) -> None:
    headers, _episode_data, _source, version = await _published_script(
        client,
        email="narrative-golden@example.com",
        body=EPISODE_THREE["source_text"],
    )

    fetched = await client.get(
        f"/api/v1/script-versions/{version['id']}/narrative-structure",
        headers=headers,
    )
    assert fetched.status_code == 200
    structure = fetched.json()["data"]
    assert structure["revision"] == 1
    assert structure["input_hash"] == version["content_hash"]
    assert len(structure["structure_hash"]) == 64
    assert len(structure["dependency_hash"]) == 64
    assert len(structure["units"]) == len(GOLDEN_UNITS) == 20

    for actual, expected in zip(structure["units"], GOLDEN_UNITS, strict=True):
        assert actual["position"] == expected["order"]
        assert actual["kind"] == expected["kind"]
        assert actual["source_range"] == {
            "start": expected["source_start_codepoint"],
            "end": expected["source_end_codepoint"],
        }
        assert actual["exact_text"] == expected["exact_text"]
        assert UUID(actual["unit_id"])
        assert UUID(actual["id"])

    payload = _revision_payload(
        structure,
        idempotency_key="narrative-correction:golden:1",
    )
    payload["units"][17]["required_for_coverage"] = False
    corrected_response = await client.post(
        f"/api/v1/narrative-structures/{structure['id']}/revisions",
        headers=headers,
        json=payload,
    )
    assert corrected_response.status_code == 201
    corrected = corrected_response.json()["data"]["structure"]
    assert corrected["revision"] == 2
    assert corrected["structure_hash"] != structure["structure_hash"]
    assert corrected["dependency_hash"] != structure["dependency_hash"]
    assert corrected["units"][17]["required_for_coverage"] is False
    assert [unit["unit_id"] for unit in corrected["units"]] == [
        unit["unit_id"] for unit in structure["units"]
    ]
    assert {unit["id"] for unit in corrected["units"]}.isdisjoint(
        {unit["id"] for unit in structure["units"]}
    )

    replay = await client.post(
        f"/api/v1/narrative-structures/{structure['id']}/revisions",
        headers=headers,
        json=payload,
    )
    assert replay.status_code == 201
    assert replay.json()["data"]["structure"] == corrected

    historical = await client.get(
        f"/api/v1/script-versions/{version['id']}/narrative-structure",
        headers=headers,
        params={"revision": 1},
    )
    assert historical.status_code == 200
    assert historical.json()["data"] == structure


@pytest.mark.asyncio
async def test_correction_rejects_overlap_cross_version_and_concurrent_revision(
    client: httpx.AsyncClient,
) -> None:
    headers, episode, source, first_version = await _published_script(
        client,
        email="narrative-conflict@example.com",
        body="第一集\n《门禁》\n内景·控制室·夜\n林澈：立即封锁闸门。\n警报灯连续闪烁。",
    )
    first_structure_response = await client.get(
        f"/api/v1/script-versions/{first_version['id']}/narrative-structure",
        headers=headers,
    )
    assert first_structure_response.status_code == 200
    first_structure = first_structure_response.json()["data"]

    second_publish = await client.post(
        f"/api/v1/script-sources/{source['id']}/versions",
        headers=headers,
        json={
            "body": (
                "第一集\n《门禁》\n内景·控制室·深夜\n林澈：立刻封锁全部闸门。\n红色警报灯连续闪烁。"
            ),
            "expected_current_version_id": first_version["id"],
        },
    )
    assert second_publish.status_code == 201
    second_version = second_publish.json()["data"]["version"]
    impact = second_publish.json()["data"]["current"]["impact"]
    assert impact["previous_narrative_dependency_hash"] == first_structure["dependency_hash"]
    assert impact["current_narrative_dependency_hash"] != first_structure["dependency_hash"]
    assert impact["invalidated_scopes"] == ["shot_readiness", "coverage", "export"]
    assert UUID(impact["narrative_impact_id"])

    dependency = await client.get(
        f"/api/v1/episodes/{episode['id']}/narrative-dependency",
        headers=headers,
        params={"evaluation_hash": first_structure["dependency_hash"]},
    )
    assert dependency.status_code == 200
    assert dependency.json()["data"]["status"] == "stale"
    assert (
        dependency.json()["data"]["current_dependency_hash"]
        == impact["current_narrative_dependency_hash"]
    )

    second_structure_response = await client.get(
        f"/api/v1/script-versions/{second_version['id']}/narrative-structure",
        headers=headers,
    )
    assert second_structure_response.status_code == 200
    second_structure = second_structure_response.json()["data"]

    cross_version = _revision_payload(
        second_structure,
        idempotency_key="narrative-cross-version",
    )
    cross_version["units"][0]["unit_id"] = first_structure["units"][0]["unit_id"]
    rejected_cross_version = await client.post(
        f"/api/v1/narrative-structures/{second_structure['id']}/revisions",
        headers=headers,
        json=cross_version,
    )
    assert rejected_cross_version.status_code == 409
    assert rejected_cross_version.json()["error"]["code"] == "resource_conflict"

    overlap = _revision_payload(
        second_structure,
        idempotency_key="narrative-overlap",
    )
    overlap["units"][1]["source_start"] = overlap["units"][0]["source_start"]
    rejected_overlap = await client.post(
        f"/api/v1/narrative-structures/{second_structure['id']}/revisions",
        headers=headers,
        json=overlap,
    )
    assert rejected_overlap.status_code == 422
    assert rejected_overlap.json()["error"]["code"] == "invalid_request"

    incomplete = _revision_payload(
        second_structure,
        idempotency_key="narrative-incomplete",
    )
    incomplete["units"][-1]["source_end"] -= 1
    rejected_incomplete = await client.post(
        f"/api/v1/narrative-structures/{second_structure['id']}/revisions",
        headers=headers,
        json=incomplete,
    )
    assert rejected_incomplete.status_code == 422
    assert rejected_incomplete.json()["error"]["details"] == {
        "missing_codepoints": 1,
        "unexpected_codepoints": 0,
    }

    first_command = _revision_payload(
        second_structure,
        idempotency_key="narrative-concurrent:first",
    )
    second_command = deepcopy(first_command)
    second_command["idempotency_key"] = "narrative-concurrent:second"
    first_command["units"][-1]["required_for_coverage"] = False
    second_command["units"][0]["required_for_coverage"] = False
    first_result, second_result = await asyncio.gather(
        client.post(
            f"/api/v1/narrative-structures/{second_structure['id']}/revisions",
            headers=headers,
            json=first_command,
        ),
        client.post(
            f"/api/v1/narrative-structures/{second_structure['id']}/revisions",
            headers=headers,
            json=second_command,
        ),
    )
    assert sorted([first_result.status_code, second_result.status_code]) == [201, 409]
    conflict = first_result if first_result.status_code == 409 else second_result
    assert conflict.json()["error"]["code"] == "version_conflict"

    latest_impact = await client.get(
        f"/api/v1/episodes/{episode['id']}/narrative-impacts/latest",
        headers=headers,
    )
    assert latest_impact.status_code == 200
    assert latest_impact.json()["data"]["trigger"] == "structure_corrected"


def _minimal_spec(version_id: UUID, scene_id: UUID) -> ShotSpec:
    return ShotSpec.model_validate(
        {
            "schema_version": 1,
            "script_reference": {
                "confirmed_script_version_id": str(version_id),
                "scene_id": str(scene_id),
                "dialogue_ids": [],
            },
            "narrative": {"purpose": "固定旧剧本镜头"},
            "visual": {
                "shot_size": "medium",
                "camera_angle": "eye_level",
                "camera_movement": "static",
                "composition": "控制室中央构图",
                "environment": "雨夜控制室",
                "subject_placements": [],
                "mood_lighting": "红色警报灯",
            },
            "action_beats": [{"beat_key": "beat-1", "order": 1, "description": "警报灯闪烁"}],
            "dialogue_or_narration": [],
            "duration_ms": 4_000,
            "generation_intent": {"mode": "text_to_video"},
        }
    )


@pytest.mark.asyncio
async def test_current_switch_adds_explicit_narrative_blocker_to_old_shot(
    client: httpx.AsyncClient,
    session_factory: async_sessionmaker[AsyncSession],
) -> None:
    body = "内景·控制室·夜\n警报灯闪烁。"
    headers, episode, source, first_version = await _published_script(
        client,
        email="narrative-readiness@example.com",
        body=body,
    )
    scene_id = uuid7()
    shot_id = uuid7()
    spec_id = uuid7()
    spec = _minimal_spec(UUID(first_version["id"]), scene_id)
    hashes = storyboard_content_hashes(spec, [])
    async with session_factory() as session, session.begin():
        stored_version = await session.get(ScriptVersion, UUID(first_version["id"]))
        assert stored_version is not None
        stored_version.structure_summary = {"confirmation_batch_id": str(uuid7())}
        session.add(
            Scene(
                id=scene_id,
                workspace_id=stored_version.workspace_id,
                script_version_id=stored_version.id,
                position=1,
                heading="内景·控制室·夜",
                location="控制室",
                time_of_day="夜",
                summary="警报灯闪烁",
                source_start=0,
                source_end=len(body),
            )
        )
        shot = Shot(
            id=shot_id,
            workspace_id=stored_version.workspace_id,
            episode_id=UUID(episode["id"]),
            position=1,
            title="旧剧本镜头",
            source_script_version_id=stored_version.id,
            source_scene_id=scene_id,
            source_candidate_id=None,
            creation_key="narrative-readiness-shot",
            status="active",
            current_spec_version_id=spec_id,
            revision=1,
            created_by=stored_version.created_by,
        )
        session.add(shot)
        await session.flush()
        session.add(
            ShotSpecVersion(
                id=spec_id,
                workspace_id=stored_version.workspace_id,
                shot_id=shot_id,
                version_no=1,
                schema_version=1,
                spec=spec.model_dump(mode="json"),
                content_hash=hashes.content_hash,
                input_hash=hashes.input_hash,
                created_by=stored_version.created_by,
            )
        )

    before = await client.get(
        f"/api/v1/shots/{shot_id}/readiness",
        headers=headers,
    )
    assert before.status_code == 200
    assert "SCRIPT_REVISION_NOT_CURRENT" not in {
        item["code"] for item in before.json()["data"]["blocking_reasons"]
    }

    second_publish = await client.post(
        f"/api/v1/script-sources/{source['id']}/versions",
        headers=headers,
        json={
            "body": "内景·控制室·深夜\n红色警报灯熄灭。",
            "expected_current_version_id": first_version["id"],
        },
    )
    assert second_publish.status_code == 201

    after = await client.get(
        f"/api/v1/shots/{shot_id}/readiness",
        headers=headers,
    )
    assert after.status_code == 200
    result = after.json()["data"]
    assert result["ready"] is False
    assert "SCRIPT_REVISION_NOT_CURRENT" in {item["code"] for item in result["blocking_reasons"]}
    assert (
        result["evaluated_dependencies"]["current_script_version_id"]
        == (second_publish.json()["data"]["version"]["id"])
    )
    assert (
        result["evaluated_dependencies"]["narrative_dependency_hash"]
        == (second_publish.json()["data"]["current"]["impact"]["current_narrative_dependency_hash"])
    )
