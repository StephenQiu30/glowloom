from collections.abc import AsyncIterator
from decimal import Decimal
from uuid import UUID

import httpx
import pytest
from pydantic import SecretStr
from sqlalchemy.exc import IntegrityError
from sqlalchemy.ext.asyncio import AsyncSession, async_sessionmaker
from uuid6 import uuid7

from app.core.config import Settings
from app.core.database import Base, create_engine, get_async_session, validate_test_database_url
from app.main import create_app
from app.modules.projects.models import Episode

TEST_DATABASE_URL = validate_test_database_url(
    "postgresql+asyncpg://postgres@127.0.0.1:5432/lanverse_test",
    "postgresql+asyncpg://postgres@127.0.0.1:5432/lanverse",
)


@pytest.fixture
async def session_factory() -> AsyncIterator[async_sessionmaker[AsyncSession]]:
    engine = create_engine(TEST_DATABASE_URL)
    async with engine.begin() as connection:
        await connection.run_sync(Base.metadata.drop_all)
        await connection.run_sync(Base.metadata.create_all)
    factory = async_sessionmaker(engine, expire_on_commit=False)
    try:
        yield factory
    finally:
        async with engine.begin() as connection:
            await connection.run_sync(Base.metadata.drop_all)
        await engine.dispose()


@pytest.fixture
async def client(
    session_factory: async_sessionmaker[AsyncSession],
) -> AsyncIterator[httpx.AsyncClient]:
    async def _test_session() -> AsyncIterator[AsyncSession]:
        async with session_factory() as session:
            yield session

    app = create_app(
        Settings(
            environment="test",
            database_url=TEST_DATABASE_URL,
            jwt_secret_key=SecretStr("project-test-secret-with-at-least-32-bytes"),
        )
    )
    app.dependency_overrides[get_async_session] = _test_session
    async with httpx.AsyncClient(
        transport=httpx.ASGITransport(app=app), base_url="http://test"
    ) as test_client:
        yield test_client


async def _identity(
    client: httpx.AsyncClient,
    *,
    email: str = "project-owner@example.com",
) -> tuple[dict[str, str], str]:
    response = await client.post(
        "/api/v1/auth/register",
        json={
            "email": email,
            "password": "a-secure-project-password",
            "display_name": "项目负责人",
        },
    )
    data = response.json()["data"]
    return {"authorization": f"Bearer {data['access_token']}"}, data["workspace"]["id"]


def _project_payload(workspace_id: str, name: str = "竖屏短剧") -> dict[str, object]:
    return {
        "workspace_id": workspace_id,
        "name": name,
        "description": "一部用于验收的短剧",
        "aspect_ratio": "9:16",
        "language": "zh-CN",
        "visual_style": "写实电影感",
        "target_duration_ms": 90000,
    }


@pytest.mark.asyncio
async def test_project_crud_budget_and_lifecycle_use_explicit_revisions(
    client: httpx.AsyncClient,
) -> None:
    headers, workspace_id = await _identity(client)
    created = await client.post(
        "/api/v1/projects",
        headers=headers,
        json=_project_payload(workspace_id),
    )
    assert created.status_code == 201
    project = created.json()["data"]
    project_id = project["id"]
    assert project["revision"] == 1
    assert project["budget_limit"] == "0.000000"
    assert project["currency"] == "CNY"

    budget_in_patch = await client.patch(
        f"/api/v1/projects/{project_id}",
        headers=headers,
        json={"name": "非法夹带", "budget_limit": "100", "expected_revision": 1},
    )
    assert budget_in_patch.status_code == 422

    updated = await client.patch(
        f"/api/v1/projects/{project_id}",
        headers=headers,
        json={
            "name": "第一季",
            "description": "修改后的简介",
            "aspect_ratio": "9:16",
            "language": "zh-CN",
            "visual_style": "水墨写实",
            "target_duration_ms": 120000,
            "expected_revision": 1,
        },
    )
    assert updated.status_code == 200
    assert updated.json()["data"]["revision"] == 2

    stale_budget = await client.post(
        f"/api/v1/projects/{project_id}/budget-limit",
        headers=headers,
        json={"amount": "99.990000", "currency": "CNY", "expected_revision": 1},
    )
    assert stale_budget.status_code == 409

    budget = await client.post(
        f"/api/v1/projects/{project_id}/budget-limit",
        headers=headers,
        json={"amount": "99.990000", "currency": "CNY", "expected_revision": 2},
    )
    assert budget.status_code == 200
    assert Decimal(budget.json()["data"]["budget_limit"]) == Decimal("99.990000")
    assert budget.json()["data"]["revision"] == 3

    archived = await client.post(
        f"/api/v1/projects/{project_id}/archive",
        headers=headers,
        json={"expected_revision": 3},
    )
    assert archived.status_code == 200
    assert archived.json()["data"]["status"] == "archived"

    blocked_update = await client.patch(
        f"/api/v1/projects/{project_id}",
        headers=headers,
        json={"name": "不应写入", "expected_revision": 4},
    )
    assert blocked_update.status_code == 409

    restored = await client.post(
        f"/api/v1/projects/{project_id}/restore",
        headers=headers,
        json={"expected_revision": 4},
    )
    assert restored.status_code == 200
    assert restored.json()["data"]["status"] == "active"


@pytest.mark.asyncio
async def test_project_lists_are_bounded_and_cross_workspace_is_hidden(
    client: httpx.AsyncClient,
) -> None:
    headers, workspace_id = await _identity(client)
    for name in ("Beta", "Alpha"):
        response = await client.post(
            "/api/v1/projects",
            headers=headers,
            json=_project_payload(workspace_id, name),
        )
        assert response.status_code == 201
    listed = await client.get(
        "/api/v1/projects",
        headers=headers,
        params={"workspace_id": workspace_id, "sort": "name", "order": "asc", "limit": 1},
    )
    assert listed.status_code == 200
    assert listed.json()["data"]["total"] == 2
    assert [item["name"] for item in listed.json()["data"]["items"]] == ["Alpha"]

    project_id = listed.json()["data"]["items"][0]["id"]
    other_headers, _ = await _identity(client, email="other-owner@example.com")
    hidden = await client.get(f"/api/v1/projects/{project_id}", headers=other_headers)
    assert hidden.status_code == 404


@pytest.mark.asyncio
async def test_empty_project_can_be_preflighted_and_deleted(
    client: httpx.AsyncClient,
) -> None:
    headers, workspace_id = await _identity(client)
    created = await client.post(
        "/api/v1/projects",
        headers=headers,
        json=_project_payload(workspace_id),
    )
    project_id = created.json()["data"]["id"]

    preflight = await client.post(
        f"/api/v1/projects/{project_id}/delete-preflight",
        headers=headers,
    )
    assert preflight.status_code == 200
    assert preflight.json()["data"] == {"allowed": True, "blockers": []}

    deleted = await client.delete(
        f"/api/v1/projects/{project_id}",
        headers=headers,
        params={"expected_revision": 1},
    )
    assert deleted.status_code == 200
    assert deleted.json()["data"]["deleted"] is True
    assert (await client.get(f"/api/v1/projects/{project_id}", headers=headers)).status_code == 404


@pytest.mark.asyncio
async def test_episode_positions_reorder_and_parent_guards_are_atomic(
    client: httpx.AsyncClient,
) -> None:
    headers, workspace_id = await _identity(client)
    project = await client.post(
        "/api/v1/projects", headers=headers, json=_project_payload(workspace_id)
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
    headers, workspace_id = await _identity(client)
    project = await client.post(
        "/api/v1/projects", headers=headers, json=_project_payload(workspace_id)
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
    headers, workspace_id = await _identity(client)
    project = await client.post(
        "/api/v1/projects", headers=headers, json=_project_payload(workspace_id)
    )
    project_id = project.json()["data"]["id"]
    _, other_workspace_id = await _identity(client, email="foreign-workspace@example.com")

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
async def test_empty_production_snapshot_explains_the_next_action(
    client: httpx.AsyncClient,
) -> None:
    headers, workspace_id = await _identity(client)
    project = await client.post(
        "/api/v1/projects", headers=headers, json=_project_payload(workspace_id)
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
