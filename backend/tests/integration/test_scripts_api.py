import asyncio
from collections.abc import AsyncIterator

import httpx
import pytest
from pydantic import SecretStr
from sqlalchemy import func, select
from sqlalchemy.ext.asyncio import AsyncSession, async_sessionmaker

from app.core.config import Settings
from app.core.database import Base, create_engine, get_async_session, validate_test_database_url
from app.main import create_app
from app.modules.messaging.models import OutboxEvent
from app.modules.scripts.models import ScriptSource, ScriptVersion

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
            jwt_secret_key=SecretStr("script-test-secret-with-at-least-32-bytes"),
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
    email: str,
) -> tuple[dict[str, str], str]:
    response = await client.post(
        "/api/v1/auth/register",
        json={
            "email": email,
            "password": "a-secure-script-password",
            "display_name": "剧本负责人",
        },
    )
    assert response.status_code == 201
    data = response.json()["data"]
    return {"authorization": f"Bearer {data['access_token']}"}, data["workspace"]["id"]


async def _episode(
    client: httpx.AsyncClient,
    headers: dict[str, str],
    workspace_id: str,
) -> dict[str, object]:
    project_response = await client.post(
        "/api/v1/projects",
        headers=headers,
        json={
            "workspace_id": workspace_id,
            "name": "剧本验收项目",
            "aspect_ratio": "9:16",
            "language": "zh-CN",
            "target_duration_ms": 90000,
        },
    )
    assert project_response.status_code == 201
    project = project_response.json()["data"]
    episode_response = await client.post(
        f"/api/v1/projects/{project['id']}/episodes",
        headers=headers,
        json={"name": "第一集", "target_duration_ms": 90000},
    )
    assert episode_response.status_code == 201
    return episode_response.json()["data"]


def _import_payload(*, body: str = "第一场\r\n角色甲：开始吧。") -> dict[str, str]:
    return {
        "input_type": "text",
        "title": "第一集原始剧本",
        "body": body,
        "rights_declaration": "确认拥有该测试文本的使用权",
        "idempotency_key": "script-import-001",
    }


@pytest.mark.asyncio
async def test_text_import_is_idempotent_private_and_creates_immutable_version(
    client: httpx.AsyncClient,
    session_factory: async_sessionmaker[AsyncSession],
) -> None:
    headers, workspace_id = await _identity(
        client, email="script-owner@example.com"
    )
    episode = await _episode(client, headers, workspace_id)
    endpoint = f"/api/v1/episodes/{episode['id']}/script-sources"

    created = await client.post(endpoint, headers=headers, json=_import_payload())
    assert created.status_code == 201
    imported = created.json()["data"]
    source = imported["source"]
    version = imported["version"]
    assert source["episode_id"] == episode["id"]
    assert source["input_type"] == "text"
    assert source["status"] == "active"
    assert source["revision"] == 1
    assert "idempotency_key" not in source
    assert version["source_id"] == source["id"]
    assert version["version_no"] == 1
    assert version["status"] == "draft"
    assert version["body"] == "第一场\n角色甲：开始吧。"
    assert len(version["content_hash"]) == 64

    repeated = await client.post(
        endpoint,
        headers=headers,
        json=_import_payload(body="第一场\n角色甲：开始吧。"),
    )
    assert repeated.status_code == 201
    assert repeated.json()["data"] == imported

    conflicting = await client.post(
        endpoint,
        headers=headers,
        json=_import_payload(body="同一个键不能替换成另一份正文"),
    )
    assert conflicting.status_code == 409
    assert conflicting.json()["error"]["code"] == "resource_conflict"

    fetched_source = await client.get(
        f"/api/v1/script-sources/{source['id']}", headers=headers
    )
    assert fetched_source.status_code == 200
    assert fetched_source.json()["data"] == source

    history = await client.get(
        f"/api/v1/script-sources/{source['id']}/versions",
        headers=headers,
    )
    assert history.status_code == 200
    assert history.json()["data"]["total"] == 1
    assert history.json()["data"]["items"] == [version]

    fetched = await client.get(
        f"/api/v1/script-versions/{version['id']}", headers=headers
    )
    assert fetched.status_code == 200
    assert fetched.json()["data"] == version
    immutable = await client.patch(
        f"/api/v1/script-versions/{version['id']}",
        headers=headers,
        json={"body": "不允许覆盖"},
    )
    assert immutable.status_code == 405

    too_long = await client.post(
        endpoint,
        headers=headers,
        json=_import_payload(body="字" * 20_001)
        | {"idempotency_key": "too-long"},
    )
    assert too_long.status_code == 422
    blank = await client.post(
        endpoint,
        headers=headers,
        json=_import_payload(body=" \r\n ") | {"idempotency_key": "blank"},
    )
    assert blank.status_code == 422

    async with session_factory() as session:
        assert await session.scalar(select(func.count()).select_from(ScriptSource)) == 1
        assert await session.scalar(select(func.count()).select_from(ScriptVersion)) == 1
        assert await session.scalar(select(func.count()).select_from(OutboxEvent)) == 0


@pytest.mark.asyncio
async def test_import_is_concurrency_safe_and_cross_workspace_hidden(
    client: httpx.AsyncClient,
    session_factory: async_sessionmaker[AsyncSession],
) -> None:
    owner_headers, workspace_id = await _identity(
        client, email="script-concurrent-owner@example.com"
    )
    episode = await _episode(client, owner_headers, workspace_id)
    endpoint = f"/api/v1/episodes/{episode['id']}/script-sources"

    first, second = await asyncio.gather(
        client.post(endpoint, headers=owner_headers, json=_import_payload()),
        client.post(endpoint, headers=owner_headers, json=_import_payload()),
    )
    assert first.status_code == 201
    assert second.status_code == 201
    assert first.json()["data"] == second.json()["data"]
    imported = first.json()["data"]

    stranger_headers, _ = await _identity(
        client, email="script-stranger@example.com"
    )
    hidden_source = await client.get(
        f"/api/v1/script-sources/{imported['source']['id']}",
        headers=stranger_headers,
    )
    hidden_version = await client.get(
        f"/api/v1/script-versions/{imported['version']['id']}",
        headers=stranger_headers,
    )
    assert hidden_source.status_code == 404
    assert hidden_version.status_code == 404

    async with session_factory() as session:
        assert await session.scalar(select(func.count()).select_from(ScriptSource)) == 1
        assert await session.scalar(select(func.count()).select_from(ScriptVersion)) == 1


@pytest.mark.asyncio
async def test_archived_episode_rejects_new_script_source(
    client: httpx.AsyncClient,
) -> None:
    headers, workspace_id = await _identity(
        client, email="script-archived-owner@example.com"
    )
    episode = await _episode(client, headers, workspace_id)
    archived = await client.post(
        f"/api/v1/episodes/{episode['id']}/archive",
        headers=headers,
        json={"expected_revision": episode["revision"]},
    )
    assert archived.status_code == 200

    response = await client.post(
        f"/api/v1/episodes/{episode['id']}/script-sources",
        headers=headers,
        json=_import_payload(),
    )
    assert response.status_code == 409
    assert response.json()["error"]["code"] == "state_conflict"
