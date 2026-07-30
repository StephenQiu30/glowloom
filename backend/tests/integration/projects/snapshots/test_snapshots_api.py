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
    assert episode_data["script_summary"] == {
        "status": "not_started",
        "current_version_id": None,
        "extraction_batch_id": None,
        "pending_required_candidates": 0,
    }
    assert episode_data["asset_summary"] == {
        "status": "not_started",
        "total": 0,
        "versioned": 0,
        "ready": 0,
        "draft": 0,
        "blocked": 0,
        "ready_kinds": [],
        "required_kinds": ["character", "location", "voice"],
    }
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


@pytest.mark.asyncio
async def test_snapshot_follows_current_script_and_latest_task_facts(
    client: httpx.AsyncClient,
) -> None:
    headers, workspace_id = await register_project_owner(
        client, email="snapshot-s2-owner@example.com"
    )
    project = await client.post(
        "/api/v1/projects", headers=headers, json=project_payload(workspace_id)
    )
    project_id = project.json()["data"]["id"]
    episode = await client.post(
        f"/api/v1/projects/{project_id}/episodes",
        headers=headers,
        json={"name": "资产准备集", "target_duration_ms": 90_000},
    )
    episode_id = episode.json()["data"]["id"]
    imported = await client.post(
        f"/api/v1/episodes/{episode_id}/script-sources",
        headers=headers,
        json={
            "input_type": "text",
            "title": "第一集",
            "body": "第一场\n顾清禾：开始吧。",
            "rights_declaration": "原创测试文本",
            "idempotency_key": "snapshot-script-import",
        },
    )
    source_id = imported.json()["data"]["source"]["id"]
    published = await client.post(
        f"/api/v1/script-sources/{source_id}/versions",
        headers=headers,
        json={
            "body": "第一场\n顾清禾：开始吧。",
            "expected_current_version_id": None,
        },
    )
    version_id = published.json()["data"]["version"]["id"]

    before_extraction = await client.get(
        f"/api/v1/episodes/{episode_id}/production-snapshot", headers=headers
    )
    assert before_extraction.status_code == 200
    before_data = before_extraction.json()["data"]
    assert before_data["current_stage"] == "structure_review"
    assert before_data["completion"] == 20
    assert before_data["blocking_reasons"][0]["code"] == "EXTRACTION_MISSING"
    assert before_data["next_actions"][0] == {
        "code": "start_extraction",
        "href": f"/studio/{episode_id}/script",
        "label": "开始结构提取",
    }
    assert before_data["script_summary"] == {
        "status": "published",
        "current_version_id": version_id,
        "extraction_batch_id": None,
        "pending_required_candidates": 0,
    }

    started = await client.post(
        f"/api/v1/script-versions/{version_id}/extractions",
        headers=headers,
        json={"scope": "full", "idempotency_key": "snapshot-extraction"},
    )
    assert started.status_code == 202
    batch_id = started.json()["data"]["id"]

    during_extraction = await client.get(
        f"/api/v1/episodes/{episode_id}/production-snapshot", headers=headers
    )
    assert during_extraction.status_code == 200
    during_data = during_extraction.json()["data"]
    assert during_data["current_stage"] == "structure_review"
    assert during_data["completion"] == 30
    assert during_data["blocking_reasons"][0]["code"] == "EXTRACTION_RUNNING"
    assert during_data["script_summary"] == {
        "status": "extracting",
        "current_version_id": version_id,
        "extraction_batch_id": batch_id,
        "pending_required_candidates": 0,
    }
    assert during_data["task_summary"] == {
        "status": "running",
        "running": 1,
        "failed": 0,
        "succeeded": 0,
        "unknown": 0,
    }

    project_snapshot = await client.get(
        f"/api/v1/projects/{project_id}/production-snapshot", headers=headers
    )
    assert project_snapshot.status_code == 200
    project_data = project_snapshot.json()["data"]
    assert project_data["current_stage"] == "structure_review"
    assert project_data["completion"] == 30
    assert project_data["episodes"][0]["script_summary"]["extraction_batch_id"] == batch_id
