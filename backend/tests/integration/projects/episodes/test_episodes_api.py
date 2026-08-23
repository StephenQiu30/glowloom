import asyncio
from uuid import UUID

import httpx
import pytest
from sqlalchemy import func, select
from sqlalchemy.exc import IntegrityError
from sqlalchemy.ext.asyncio import AsyncSession, async_sessionmaker
from uuid6 import uuid7

from app.modules.governance.audit.models import AuditEvent
from app.modules.identity import ActorContext
from app.modules.identity.models import Membership
from app.modules.production import ScriptExtractionTaskCommand, create_script_extraction_task
from app.modules.projects.models import Episode
from app.modules.scripts.models import ScriptSource
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

    reordered_audit = await client.get(
        "/api/v1/audit-events",
        headers=headers,
        params={
            "workspace_id": workspace_id,
            "action": "episode.reordered",
            "target_type": "project",
            "target_id": project_id,
        },
    )
    assert reordered_audit.status_code == 200
    assert reordered_audit.json()["data"]["total"] == 1
    assert reordered_audit.json()["data"]["items"][0]["metadata"] == {
        "project_revision": 5,
        "episode_count": 3,
    }

    updated_audit = await client.get(
        "/api/v1/audit-events",
        headers=headers,
        params={
            "workspace_id": workspace_id,
            "target_type": "episode",
            "target_id": episode_ids[0],
        },
    )
    assert updated_audit.status_code == 200
    assert [item["action"] for item in updated_audit.json()["data"]["items"]] == [
        "episode.updated",
        "episode.created",
    ]
    assert updated_audit.json()["data"]["items"][0]["metadata"]["changed_fields"] == [
        "name",
        "target_duration_ms",
    ]
    assert "改名后的单集" not in str(updated_audit.json()["data"])

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
    lifecycle_audit = await client.get(
        "/api/v1/audit-events",
        headers=headers,
        params={
            "workspace_id": workspace_id,
            "target_type": "episode",
            "target_id": str(episodes[0]["id"]),
        },
    )
    assert lifecycle_audit.status_code == 200
    assert [item["action"] for item in lifecycle_audit.json()["data"]["items"]] == [
        "episode.deleted",
        "episode.restored",
        "episode.archived",
        "episode.created",
    ]
    assert lifecycle_audit.json()["data"]["items"][0]["metadata"] == {
        "project_id": project_id,
        "project_revision": 6,
        "revision": 3,
        "position": 2,
        "status": "active",
    }
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


@pytest.mark.asyncio
async def test_task_created_after_preflight_blocks_episode_and_project_deletion(
    client: httpx.AsyncClient,
    session_factory: async_sessionmaker[AsyncSession],
) -> None:
    headers, workspace_id = await register_project_owner(
        client,
        email="task-delete-blocker@example.com",
    )
    project_response = await client.post(
        "/api/v1/projects",
        headers=headers,
        json=project_payload(workspace_id, "任务删除门禁"),
    )
    project = project_response.json()["data"]
    episode_response = await client.post(
        f"/api/v1/projects/{project['id']}/episodes",
        headers=headers,
        json={"name": "任务引用单集", "target_duration_ms": 90000},
    )
    episode = episode_response.json()["data"]

    initial_preflight = await client.post(
        f"/api/v1/episodes/{episode['id']}/delete-preflight",
        headers=headers,
    )
    assert initial_preflight.status_code == 200
    assert initial_preflight.json()["data"] == {"allowed": True, "blockers": []}

    async with session_factory() as session:
        async with session.begin():
            membership = await session.scalar(
                select(Membership).where(Membership.workspace_id == UUID(workspace_id))
            )
            assert membership is not None
            actor = ActorContext(
                user_id=membership.user_id,
                workspace_id=membership.workspace_id,
                membership_id=membership.id,
                role="owner",
                workspace_status="active",
            )
            await create_script_extraction_task(
                session,
                actor,
                ScriptExtractionTaskCommand(
                    workspace_id=UUID(workspace_id),
                    episode_id=UUID(episode["id"]),
                    request_id=uuid7(),
                    input_version_id=uuid7(),
                    input_hash="d" * 64,
                    idempotency_key="task-delete-blocker",
                ),
                trace_id="task-delete-blocker-trace",
            )

    episode_preflight = await client.post(
        f"/api/v1/episodes/{episode['id']}/delete-preflight",
        headers=headers,
    )
    assert episode_preflight.status_code == 200
    assert episode_preflight.json()["data"] == {
        "allowed": False,
        "blockers": [
            {
                "code": "HAS_TASKS",
                "resource_type": "episode",
                "resource_id": episode["id"],
                "summary": "单集已有 1 个任务",
            }
        ],
    }

    project_preflight = await client.post(
        f"/api/v1/projects/{project['id']}/delete-preflight",
        headers=headers,
    )
    assert project_preflight.status_code == 200
    assert project_preflight.json()["data"]["allowed"] is False
    assert {
        blocker["code"]: blocker["summary"]
        for blocker in project_preflight.json()["data"]["blockers"]
    } == {
        "HAS_EPISODES": "项目包含 1 个单集",
        "HAS_TASKS": "项目关联 1 个任务",
    }

    blocked_delete = await client.delete(
        f"/api/v1/episodes/{episode['id']}",
        headers=headers,
        params={"expected_revision": episode["revision"]},
    )
    assert blocked_delete.status_code == 409
    assert blocked_delete.json()["error"]["next_action"] == "review_delete_blockers"
    assert (
        await client.get(f"/api/v1/episodes/{episode['id']}", headers=headers)
    ).status_code == 200

    async with session_factory() as session:
        assert (
            await session.scalar(
                select(func.count())
                .select_from(AuditEvent)
                .where(
                    AuditEvent.action == "episode.deleted",
                    AuditEvent.target_id == UUID(episode["id"]),
                )
            )
            == 0
        )


@pytest.mark.asyncio
async def test_archived_script_versions_created_after_preflight_block_deletion(
    client: httpx.AsyncClient,
    session_factory: async_sessionmaker[AsyncSession],
) -> None:
    headers, workspace_id = await register_project_owner(
        client,
        email="script-delete-blocker@example.com",
    )
    project_response = await client.post(
        "/api/v1/projects",
        headers=headers,
        json=project_payload(workspace_id, "剧本版本删除门禁"),
    )
    project = project_response.json()["data"]
    episode_response = await client.post(
        f"/api/v1/projects/{project['id']}/episodes",
        headers=headers,
        json={"name": "剧本引用单集", "target_duration_ms": 90000},
    )
    episode = episode_response.json()["data"]

    initial_preflight = await client.post(
        f"/api/v1/episodes/{episode['id']}/delete-preflight",
        headers=headers,
    )
    assert initial_preflight.status_code == 200
    assert initial_preflight.json()["data"] == {"allowed": True, "blockers": []}

    imported_versions: list[dict[str, object]] = []
    imported_sources: list[dict[str, object]] = []
    for number in range(1, 3):
        imported_response = await client.post(
            f"/api/v1/episodes/{episode['id']}/script-sources",
            headers=headers,
            json={
                "input_type": "text",
                "title": f"第 {number} 份草稿",
                "body": f"第 {number} 场\n角色甲：继续。",
                "rights_declaration": "确认拥有该测试文本的使用权",
                "idempotency_key": f"script-delete-blocker-{number}",
            },
        )
        assert imported_response.status_code == 201
        imported_sources.append(imported_response.json()["data"]["source"])
        imported_versions.append(imported_response.json()["data"]["version"])

    archived_source = await client.post(
        f"/api/v1/script-sources/{imported_sources[0]['id']}/archive",
        headers=headers,
        json={"expected_revision": 1},
    )
    assert archived_source.status_code == 200
    assert archived_source.json()["data"]["status"] == "archived"

    episode_after_import = await client.get(f"/api/v1/episodes/{episode['id']}", headers=headers)
    assert episode_after_import.status_code == 200
    assert episode_after_import.json()["data"]["current_script_version_id"] is None

    episode_preflight = await client.post(
        f"/api/v1/episodes/{episode['id']}/delete-preflight",
        headers=headers,
    )
    assert episode_preflight.status_code == 200
    assert episode_preflight.json()["data"] == {
        "allowed": False,
        "blockers": [
            {
                "code": "HAS_SCRIPT_VERSIONS",
                "resource_type": "episode",
                "resource_id": episode["id"],
                "summary": "单集已有 2 个剧本版本",
            }
        ],
    }

    project_preflight = await client.post(
        f"/api/v1/projects/{project['id']}/delete-preflight",
        headers=headers,
    )
    assert project_preflight.status_code == 200
    assert project_preflight.json()["data"]["allowed"] is False
    assert {
        blocker["code"]: blocker["summary"]
        for blocker in project_preflight.json()["data"]["blockers"]
    } == {
        "HAS_EPISODES": "项目包含 1 个单集",
        "HAS_SCRIPT_VERSIONS": "项目关联 2 个剧本版本",
    }

    blocked_delete = await client.delete(
        f"/api/v1/episodes/{episode['id']}",
        headers=headers,
        params={"expected_revision": episode["revision"]},
    )
    assert blocked_delete.status_code == 409
    assert blocked_delete.json()["error"]["next_action"] == "review_delete_blockers"
    assert (
        await client.get(
            f"/api/v1/script-versions/{imported_versions[0]['id']}",
            headers=headers,
        )
    ).status_code == 200

    async with session_factory() as session:
        assert (
            await session.scalar(
                select(func.count())
                .select_from(AuditEvent)
                .where(
                    AuditEvent.action == "episode.deleted",
                    AuditEvent.target_id == UUID(episode["id"]),
                )
            )
            == 0
        )


@pytest.mark.asyncio
async def test_concurrent_script_import_and_episode_delete_never_orphan_versions(
    client: httpx.AsyncClient,
    session_factory: async_sessionmaker[AsyncSession],
) -> None:
    headers, workspace_id = await register_project_owner(
        client,
        email="script-delete-race@example.com",
    )
    project_response = await client.post(
        "/api/v1/projects",
        headers=headers,
        json=project_payload(workspace_id, "剧本删除并发门禁"),
    )
    project = project_response.json()["data"]
    episode_response = await client.post(
        f"/api/v1/projects/{project['id']}/episodes",
        headers=headers,
        json={"name": "并发争用单集", "target_duration_ms": 90000},
    )
    episode = episode_response.json()["data"]

    import_response, delete_response = await asyncio.gather(
        client.post(
            f"/api/v1/episodes/{episode['id']}/script-sources",
            headers=headers,
            json={
                "input_type": "text",
                "title": "并发导入草稿",
                "body": "第一场\n角色甲：不能留下孤立版本。",
                "rights_declaration": "确认拥有该测试文本的使用权",
                "idempotency_key": "script-delete-race",
            },
        ),
        client.delete(
            f"/api/v1/episodes/{episode['id']}",
            headers=headers,
            params={"expected_revision": episode["revision"]},
        ),
    )

    outcomes = (import_response.status_code, delete_response.status_code)
    assert outcomes in {(201, 409), (404, 200)}
    episode_after_race = await client.get(f"/api/v1/episodes/{episode['id']}", headers=headers)
    if outcomes == (201, 409):
        assert delete_response.json()["error"]["next_action"] == "review_delete_blockers"
        assert episode_after_race.status_code == 200
        imported_version_id = import_response.json()["data"]["version"]["id"]
        assert (
            await client.get(
                f"/api/v1/script-versions/{imported_version_id}",
                headers=headers,
            )
        ).status_code == 200
    else:
        assert episode_after_race.status_code == 404
        async with session_factory() as session:
            assert (
                await session.scalar(
                    select(func.count())
                    .select_from(ScriptSource)
                    .where(ScriptSource.episode_id == UUID(episode["id"]))
                )
                == 0
            )
