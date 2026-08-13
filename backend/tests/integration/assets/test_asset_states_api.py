import asyncio
from typing import Any
from uuid import UUID

import httpx
import pytest
from sqlalchemy import text
from sqlalchemy.ext.asyncio import AsyncSession, async_sessionmaker

from tests.support.identity_builders import register_identity_response
from tests.support.project_builders import project_payload


async def _project(
    client: httpx.AsyncClient,
    *,
    email: str,
) -> tuple[dict[str, str], dict[str, Any], dict[str, Any]]:
    registered = await register_identity_response(client, email=email)
    assert registered.status_code == 201
    identity = registered.json()["data"]
    headers = {"authorization": f"Bearer {identity['access_token']}"}
    created = await client.post(
        "/api/v1/projects",
        headers=headers,
        json=project_payload(identity["workspace"]["id"], "剧情状态验收项目"),
    )
    assert created.status_code == 201
    return headers, identity, created.json()["data"]


async def _asset(
    client: httpx.AsyncClient,
    headers: dict[str, str],
    project_id: str,
    *,
    name: str = "沈岚",
) -> dict[str, Any]:
    response = await client.post(
        f"/api/v1/projects/{project_id}/assets",
        headers=headers,
        json={"kind": "character", "name": name},
    )
    assert response.status_code == 201
    return response.json()["data"]


async def _state(
    client: httpx.AsyncClient,
    headers: dict[str, str],
    asset: dict[str, Any],
    *,
    state_key: str,
    label: str,
    idempotency_key: str,
) -> dict[str, Any]:
    response = await client.post(
        f"/api/v1/assets/{asset['id']}/states",
        headers=headers,
        json={
            "state_key": state_key,
            "label": label,
            "description": f"{label}剧情连续性状态",
            "expected_asset_revision": asset["revision"],
            "idempotency_key": idempotency_key,
        },
    )
    assert response.status_code == 201
    return response.json()["data"]


async def _episode_with_narrative(
    client: httpx.AsyncClient,
    headers: dict[str, str],
    project_id: str,
) -> tuple[dict[str, Any], dict[str, Any], dict[str, Any]]:
    episode_response = await client.post(
        f"/api/v1/projects/{project_id}/episodes",
        headers=headers,
        json={"name": "第三集", "target_duration_ms": 92_000},
    )
    assert episode_response.status_code == 201
    episode = episode_response.json()["data"]
    body = "内景·旧泵站·雨夜\n沈岚扶着受伤的右臂走向控制台。\n沈岚：闸门只剩三十秒。"
    source_response = await client.post(
        f"/api/v1/episodes/{episode['id']}/script-sources",
        headers=headers,
        json={
            "input_type": "text",
            "title": "剧情状态验收稿",
            "body": body,
            "rights_declaration": "原创自动化测试文本",
            "idempotency_key": f"state-script:{episode['id']}",
        },
    )
    assert source_response.status_code == 201
    source = source_response.json()["data"]["source"]
    version_response = await client.post(
        f"/api/v1/script-sources/{source['id']}/versions",
        headers=headers,
        json={"body": body, "expected_current_version_id": None},
    )
    assert version_response.status_code == 201
    version = version_response.json()["data"]["version"]
    structure_response = await client.get(
        f"/api/v1/script-versions/{version['id']}/narrative-structure",
        headers=headers,
    )
    assert structure_response.status_code == 200
    return episode, version, structure_response.json()["data"]


def _draft_version_payload(
    state: dict[str, Any],
    *,
    appearance: str,
) -> dict[str, object]:
    return {
        "spec": {
            "kind": "character",
            "identity": "沈岚",
            "appearance": appearance,
            "age_impression": "31 岁",
            "temperament": ["坚定"],
        },
        "prompt_description": appearance,
        "media_references": [],
        "source_type": "manual",
        "source_id": None,
        "expected_revision": state["revision"],
        "expected_current_version_id": state["current_version_id"],
        "set_as_current": True,
    }


@pytest.mark.asyncio
async def test_asset_owns_atomic_base_state_and_state_creation_is_idempotent(
    client: httpx.AsyncClient,
    session_factory: async_sessionmaker[AsyncSession],
) -> None:
    headers, _, project = await _project(
        client,
        email="asset-state-identity@example.com",
    )
    asset = await _asset(client, headers, project["id"])
    assert "current_version_id" not in asset

    listed = await client.get(
        f"/api/v1/assets/{asset['id']}/states",
        headers=headers,
    )
    assert listed.status_code == 200
    assert listed.json()["data"]["total"] == 1
    base = listed.json()["data"]["items"][0]
    assert base["state_key"] == "base"
    assert base["label"] == "基础状态"
    assert base["current_version_id"] is None
    assert base["revision"] == 1

    command = {
        "state_key": "injured",
        "label": "受伤",
        "description": "右臂受伤，衣袖浸湿",
        "expected_asset_revision": asset["revision"],
        "idempotency_key": "state:shen-lan:injured",
    }
    created = await client.post(
        f"/api/v1/assets/{asset['id']}/states",
        headers=headers,
        json=command,
    )
    replay = await client.post(
        f"/api/v1/assets/{asset['id']}/states",
        headers=headers,
        json=command,
    )
    assert created.status_code == replay.status_code == 201
    assert replay.json()["data"] == created.json()["data"]
    assert created.json()["data"]["asset"]["revision"] == asset["revision"] + 1
    assert created.json()["data"]["state"]["state_key"] == "injured"

    collision = await client.post(
        f"/api/v1/assets/{asset['id']}/states",
        headers=headers,
        json={**command, "idempotency_key": "state:collision"},
    )
    assert collision.status_code == 409
    assert collision.json()["error"]["code"] == "resource_conflict"

    async with session_factory() as session:
        state_rows = (
            await session.execute(
                text(
                    "SELECT state_key, current_version_id FROM ast_asset_states "
                    "WHERE asset_id = :asset_id ORDER BY state_key"
                ),
                {"asset_id": UUID(asset["id"])},
            )
        ).all()
    assert state_rows == [("base", None), ("injured", None)]


@pytest.mark.asyncio
async def test_state_current_is_cas_scoped_and_asset_current_api_is_removed(
    client: httpx.AsyncClient,
) -> None:
    headers, _, project = await _project(client, email="state-current@example.com")
    asset = await _asset(client, headers, project["id"])
    states = await client.get(f"/api/v1/assets/{asset['id']}/states", headers=headers)
    base = states.json()["data"]["items"][0]
    injured_result = await _state(
        client,
        headers,
        asset,
        state_key="injured",
        label="受伤",
        idempotency_key="state-current:injured",
    )
    injured = injured_result["state"]

    base_created = await client.post(
        f"/api/v1/asset-states/{base['id']}/versions",
        headers=headers,
        json=_draft_version_payload(base, appearance="深蓝常服，状态完好"),
    )
    assert base_created.status_code == 201
    base_version = base_created.json()["data"]["version"]
    assert base_created.json()["data"]["state"]["current_version_id"] == base_version["id"]

    injured_created = await client.post(
        f"/api/v1/asset-states/{injured['id']}/versions",
        headers=headers,
        json=_draft_version_payload(injured, appearance="右臂受伤，衣袖浸湿"),
    )
    assert injured_created.status_code == 201
    injured_state = injured_created.json()["data"]["state"]
    injured_version = injured_created.json()["data"]["version"]
    assert injured_version["asset_state_id"] == injured["id"]

    cross_state = await client.post(
        f"/api/v1/asset-states/{injured['id']}/current-version",
        headers=headers,
        json={
            "version_id": base_version["id"],
            "expected_current_version_id": injured_version["id"],
            "expected_revision": injured_state["revision"],
            "idempotency_key": "state-current:cross-state",
        },
    )
    assert cross_state.status_code == 409
    assert cross_state.json()["error"]["code"] == "resource_conflict"

    first, second = await asyncio.gather(
        client.post(
            f"/api/v1/asset-states/{injured['id']}/versions",
            headers=headers,
            json=_draft_version_payload(injured_state, appearance="受伤后披上雨衣"),
        ),
        client.post(
            f"/api/v1/asset-states/{injured['id']}/versions",
            headers=headers,
            json=_draft_version_payload(injured_state, appearance="受伤后衣袖破损"),
        ),
    )
    assert sorted([first.status_code, second.status_code]) == [201, 409]

    removed = await client.post(
        f"/api/v1/assets/{asset['id']}/current-version",
        headers=headers,
        json={
            "version_id": base_version["id"],
            "expected_current_version_id": None,
            "expected_revision": 1,
        },
    )
    assert removed.status_code == 404


@pytest.mark.asyncio
async def test_occurrence_decisions_are_append_only_scoped_and_staleness_visible(
    client: httpx.AsyncClient,
    session_factory: async_sessionmaker[AsyncSession],
) -> None:
    headers, _, project = await _project(client, email="state-occurrence@example.com")
    asset = await _asset(client, headers, project["id"])
    created_state = await _state(
        client,
        headers,
        asset,
        state_key="injured",
        label="受伤",
        idempotency_key="occurrence:state",
    )
    state = created_state["state"]
    episode, _, structure = await _episode_with_narrative(
        client,
        headers,
        project["id"],
    )
    unit = structure["units"][1]
    command = {
        "decision": "link",
        "narrative_unit_id": unit["unit_id"],
        "narrative_unit_version_id": unit["id"],
        "expected_revision": state["revision"],
        "idempotency_key": "occurrence:injured:unit-2:link",
    }
    linked = await client.post(
        f"/api/v1/asset-states/{state['id']}/occurrence-decisions",
        headers=headers,
        json=command,
    )
    replay = await client.post(
        f"/api/v1/asset-states/{state['id']}/occurrence-decisions",
        headers=headers,
        json=command,
    )
    assert linked.status_code == replay.status_code == 201
    assert replay.json()["data"] == linked.json()["data"]
    linked_data = linked.json()["data"]
    assert linked_data["decision"]["episode_id"] == episode["id"]
    assert linked_data["decision"]["sequence"] == 1
    assert linked_data["state"]["revision"] == state["revision"] + 1

    current = await client.get(
        f"/api/v1/asset-states/{state['id']}/occurrences",
        headers=headers,
    )
    assert current.status_code == 200
    assert current.json()["data"]["total"] == 1
    assert current.json()["data"]["items"][0]["freshness"] == "current"

    corrected_payload = {
        "expected_revision": structure["revision"],
        "expected_current_script_version_id": structure["script_version_id"],
        "idempotency_key": "occurrence:narrative-correction",
        "units": [
            {
                "unit_id": item["unit_id"],
                "kind": item["kind"],
                "source_start": item["source_range"]["start"],
                "source_end": item["source_range"]["end"],
                "required_for_coverage": item["required_for_coverage"],
            }
            for item in structure["units"]
        ],
    }
    corrected_payload["units"][1]["required_for_coverage"] = False
    corrected = await client.post(
        f"/api/v1/narrative-structures/{structure['id']}/revisions",
        headers=headers,
        json=corrected_payload,
    )
    assert corrected.status_code == 201

    stale = await client.get(
        f"/api/v1/asset-states/{state['id']}/occurrences",
        headers=headers,
    )
    assert stale.status_code == 200
    assert stale.json()["data"]["items"][0]["freshness"] == "stale"

    current_state = linked_data["state"]
    unlinked = await client.post(
        f"/api/v1/asset-states/{state['id']}/occurrence-decisions",
        headers=headers,
        json={
            "decision": "unlink",
            "narrative_unit_id": unit["unit_id"],
            "narrative_unit_version_id": corrected.json()["data"]["structure"]["units"][1]["id"],
            "expected_revision": current_state["revision"],
            "idempotency_key": "occurrence:injured:unit-2:unlink",
        },
    )
    assert unlinked.status_code == 201
    active_after_unlink = await client.get(
        f"/api/v1/asset-states/{state['id']}/occurrences",
        headers=headers,
    )
    history = await client.get(
        f"/api/v1/asset-states/{state['id']}/occurrences",
        headers=headers,
        params={"include_history": True},
    )
    assert active_after_unlink.json()["data"]["total"] == 0
    assert [item["decision"] for item in history.json()["data"]["items"]] == [
        "link",
        "unlink",
    ]

    async with session_factory() as session:
        stored = await session.execute(
            text(
                "SELECT decision, sequence FROM ast_asset_occurrences "
                "WHERE asset_state_id = :state_id ORDER BY sequence"
            ),
            {"state_id": UUID(state["id"])},
        )
    assert stored.all() == [("link", 1), ("unlink", 2)]


@pytest.mark.asyncio
async def test_asset_bible_groups_state_occurrences_and_readiness(
    client: httpx.AsyncClient,
) -> None:
    headers, _, project = await _project(client, email="asset-bible@example.com")
    asset = await _asset(client, headers, project["id"])
    injured_result = await _state(
        client,
        headers,
        asset,
        state_key="injured",
        label="受伤",
        idempotency_key="bible:injured",
    )
    injured = injured_result["state"]
    episode, _, structure = await _episode_with_narrative(
        client,
        headers,
        project["id"],
    )
    unit = structure["units"][1]
    linked = await client.post(
        f"/api/v1/asset-states/{injured['id']}/occurrence-decisions",
        headers=headers,
        json={
            "decision": "link",
            "narrative_unit_id": unit["unit_id"],
            "narrative_unit_version_id": unit["id"],
            "expected_revision": injured["revision"],
            "idempotency_key": "bible:injured:unit",
        },
    )
    assert linked.status_code == 201

    bible = await client.get(
        f"/api/v1/projects/{project['id']}/asset-bible",
        headers=headers,
        params={
            "purpose": "ai_short_drama_generation",
            "channel": "lanverse_preview",
            "region": "CN",
        },
    )
    assert bible.status_code == 200
    data = bible.json()["data"]
    assert data["summary"] == {
        "asset_count": 1,
        "state_count": 2,
        "ready": 0,
        "draft": 2,
        "blocked": 0,
        "unavailable": 0,
    }
    assert len(data["items"]) == 1
    assert data["items"][0]["asset"]["id"] == asset["id"]
    states = {item["state"]["state_key"]: item for item in data["items"][0]["states"]}
    assert set(states) == {"base", "injured"}
    assert states["base"]["readiness"]["blockers"][0]["code"] == (
        "state_current_version_missing"
    )
    assert states["injured"]["occurrences"][0]["episode_id"] == episode["id"]
    assert states["injured"]["readiness"]["warnings"] == []


@pytest.mark.asyncio
async def test_occurrence_rejects_narrative_unit_from_another_project(
    client: httpx.AsyncClient,
) -> None:
    headers, identity, first_project = await _project(
        client,
        email="occurrence-scope@example.com",
    )
    asset = await _asset(client, headers, first_project["id"])
    state_result = await _state(
        client,
        headers,
        asset,
        state_key="injured",
        label="受伤",
        idempotency_key="scope:state",
    )
    second_project_response = await client.post(
        "/api/v1/projects",
        headers=headers,
        json=project_payload(identity["workspace"]["id"], "另一个项目"),
    )
    assert second_project_response.status_code == 201
    _, _, foreign_structure = await _episode_with_narrative(
        client,
        headers,
        second_project_response.json()["data"]["id"],
    )
    foreign_unit = foreign_structure["units"][0]
    rejected = await client.post(
        f"/api/v1/asset-states/{state_result['state']['id']}/occurrence-decisions",
        headers=headers,
        json={
            "decision": "link",
            "narrative_unit_id": foreign_unit["unit_id"],
            "narrative_unit_version_id": foreign_unit["id"],
            "expected_revision": state_result["state"]["revision"],
            "idempotency_key": "scope:foreign-unit",
        },
    )
    assert rejected.status_code == 409
    assert rejected.json()["error"]["code"] == "resource_conflict"
