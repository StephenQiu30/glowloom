import httpx
import pytest

from tests.support.project_builders import project_payload, register_project_owner


@pytest.mark.asyncio
async def test_empty_production_snapshot_explains_the_next_action(
    client: httpx.AsyncClient,
) -> None:
    headers, workspace_id = await register_project_owner(client)
    project = await client.post(
        "/api/v1/projects", headers=headers, json=project_payload(workspace_id)
    )
    project_id = project.json()["data"]["id"]
    empty_project_snapshot = await client.get(
        f"/api/v1/projects/{project_id}/production-snapshot", headers=headers
    )
    assert empty_project_snapshot.json()["data"]["blocking_reasons"][0] == {
        "code": "EPISODE_MISSING",
        "resource_id": project_id,
        "resource_type": "project",
        "summary": "项目尚未创建有效单集",
    }
    episode = await client.post(
        f"/api/v1/projects/{project_id}/episodes",
        headers=headers,
        json={"name": "试播集", "target_duration_ms": 90000},
    )
    episode_id = episode.json()["data"]["id"]

    episode_snapshot = await client.get(
        f"/api/v1/episodes/{episode_id}/production-snapshot", headers=headers
    )
    assert episode_snapshot.status_code == 200
    episode_data = episode_snapshot.json()["data"]
    assert episode_data["current_stage"] == "script_import"
    assert episode_data["completion"] == 0
    assert episode_data["blocking_reasons"][0]["code"] == "SCRIPT_MISSING"
    assert episode_data["blocking_reasons"][0]["summary"] == "单集尚未导入剧本"
    assert episode_data["next_actions"][0] == {
        "code": "import_script",
        "href": f"/studio/{episode_id}/script",
        "label": "导入剧本",
    }
    assert episode_data["partial_failures"] == []
    assert episode_data["cost_summary"] == {
        "currency": "CNY",
        "reserved": "0.000000",
        "used": "0.000000",
        "status": "not_started",
    }

    project_snapshot = await client.get(
        f"/api/v1/projects/{project_id}/production-snapshot", headers=headers
    )
    assert project_snapshot.status_code == 200
    assert project_snapshot.json()["data"]["episodes"][0]["episode_id"] == episode_id
    assert project_snapshot.json()["data"]["current_stage"] == "script_import"

    not_editable = await client.patch(
        f"/api/v1/episodes/{episode_id}/production-snapshot",
        headers=headers,
        json={"current_stage": "done"},
    )
    assert not_editable.status_code == 405
