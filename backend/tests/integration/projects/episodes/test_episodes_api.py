from uuid import UUID

import httpx
import pytest
from sqlalchemy.exc import IntegrityError
from sqlalchemy.ext.asyncio import AsyncSession, async_sessionmaker
from uuid6 import uuid7

from app.modules.projects.models import Episode
from tests.support.project_builders import project_payload, register_project_owner


@pytest.mark.asyncio
async def test_episode_positions_reorder_and_parent_guards_are_atomic(
    client: httpx.AsyncClient,
) -> None:
    headers, workspace_id = await register_project_owner(client)
    project = await client.post(
        "/api/v1/projects", headers=headers, json=project_payload(workspace_id)
    )
    project_id = project.json()["data"]["id"]

    episode_ids: list[str] = []
    for number in range(1, 4):
        created = await client.post(
            f"/api/v1/projects/{project_id}/episodes",
            headers=headers,
            json={"name": f"第 {number} 集", "target_duration_ms": 90000},
        )
        assert created.status_code == 201
        assert created.json()["data"]["position"] == number
        episode_ids.append(created.json()["data"]["id"])

    project_after_create = await client.get(f"/api/v1/projects/{project_id}", headers=headers)
    assert project_after_create.json()["data"]["revision"] == 4

    incomplete = await client.post(
        f"/api/v1/projects/{project_id}/episodes/reorder",
        headers=headers,
        json={"episode_ids": episode_ids[:2], "expected_revision": 4},
    )
    assert incomplete.status_code == 422

    reordered = await client.post(
        f"/api/v1/projects/{project_id}/episodes/reorder",
        headers=headers,
        json={
            "episode_ids": [episode_ids[2], episode_ids[0], episode_ids[1]],
            "expected_revision": 4,
        },
    )
    assert reordered.status_code == 200
    assert reordered.json()["data"]["project_revision"] == 5
    assert [item["id"] for item in reordered.json()["data"]["items"]] == [
        episode_ids[2],
        episode_ids[0],
        episode_ids[1],
    ]
    assert [item["position"] for item in reordered.json()["data"]["items"]] == [1, 2, 3]

    updated = await client.patch(
        f"/api/v1/episodes/{episode_ids[0]}",
        headers=headers,
        json={"name": "改名后的单集", "target_duration_ms": 110000, "expected_revision": 1},
    )
    assert updated.status_code == 200
    assert updated.json()["data"]["revision"] == 2

    project_preflight = await client.post(
        f"/api/v1/projects/{project_id}/delete-preflight", headers=headers
    )
    assert project_preflight.json()["data"]["allowed"] is False
    assert project_preflight.json()["data"]["blockers"][0]["code"] == "HAS_EPISODES"
    assert project_preflight.json()["data"]["blockers"][0]["summary"] == "项目包含 3 个单集"
    blocked_delete = await client.delete(
        f"/api/v1/projects/{project_id}", headers=headers, params={"expected_revision": 5}
    )
    assert blocked_delete.status_code == 409

    archived_project = await client.post(
        f"/api/v1/projects/{project_id}/archive",
        headers=headers,
        json={"expected_revision": 5},
    )
    assert archived_project.status_code == 200
    blocked_episode = await client.post(
        f"/api/v1/projects/{project_id}/episodes",
        headers=headers,
        json={"name": "不应创建", "target_duration_ms": 90000},
    )
    assert blocked_episode.status_code == 409


@pytest.mark.asyncio
async def test_episode_archive_restore_and_empty_delete_preserve_contiguous_order(
    client: httpx.AsyncClient,
) -> None:
    headers, workspace_id = await register_project_owner(client)
    project = await client.post(
        "/api/v1/projects", headers=headers, json=project_payload(workspace_id)
    )
    project_id = project.json()["data"]["id"]
    episodes: list[dict[str, object]] = []
    for name in ("A", "B"):
        response = await client.post(
            f"/api/v1/projects/{project_id}/episodes",
            headers=headers,
            json={"name": name, "target_duration_ms": 90000},
        )
        episodes.append(response.json()["data"])

    archived = await client.post(
        f"/api/v1/episodes/{episodes[0]['id']}/archive",
        headers=headers,
        json={"expected_revision": 1},
    )
    assert archived.status_code == 200
    active = await client.get(f"/api/v1/projects/{project_id}/episodes", headers=headers)
    assert [(item["name"], item["position"]) for item in active.json()["data"]] == [("B", 1)]

    restored = await client.post(
        f"/api/v1/episodes/{episodes[0]['id']}/restore",
        headers=headers,
        json={"expected_revision": 2},
    )
    assert restored.status_code == 200
    assert restored.json()["data"]["position"] == 2

    preflight = await client.post(
        f"/api/v1/episodes/{episodes[0]['id']}/delete-preflight", headers=headers
    )
    assert preflight.json()["data"] == {"allowed": True, "blockers": []}
    deleted = await client.delete(
        f"/api/v1/episodes/{episodes[0]['id']}",
        headers=headers,
        params={"expected_revision": 3},
    )
    assert deleted.status_code == 200
    remaining = await client.get(f"/api/v1/projects/{project_id}/episodes", headers=headers)
    assert [(item["name"], item["position"]) for item in remaining.json()["data"]] == [("B", 1)]


@pytest.mark.asyncio
async def test_database_rejects_episode_with_mismatched_workspace(
    client: httpx.AsyncClient,
    session_factory: async_sessionmaker[AsyncSession],
) -> None:
    headers, workspace_id = await register_project_owner(client)
    project = await client.post(
        "/api/v1/projects", headers=headers, json=project_payload(workspace_id)
    )
    project_id = project.json()["data"]["id"]
    _, other_workspace_id = await register_project_owner(
        client, email="foreign-workspace@example.com"
    )

    async with session_factory() as session:
        with pytest.raises(IntegrityError):
            async with session.begin():
                session.add(
                    Episode(
                        id=uuid7(),
                        workspace_id=UUID(other_workspace_id),
                        project_id=UUID(project_id),
                        name="非法跨空间单集",
                        position=1,
                        target_duration_ms=90000,
                    )
                )
                await session.flush()
