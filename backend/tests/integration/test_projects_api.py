from collections.abc import AsyncIterator
from decimal import Decimal

import httpx
import pytest
from pydantic import SecretStr
from sqlalchemy.ext.asyncio import AsyncSession, async_sessionmaker

from app.core.config import Settings
from app.core.database import Base, create_engine, get_async_session, validate_test_database_url
from app.main import create_app

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
