import asyncio

import httpx
import pytest
from sqlalchemy import func, select
from sqlalchemy.ext.asyncio import AsyncSession, async_sessionmaker

from app.modules.identity.models import Membership, UserAccount, Workspace


async def _register(client: httpx.AsyncClient) -> httpx.Response:
    return await client.post(
        "/api/v1/auth/register",
        json={
            "email": "Creator@Example.com",
            "password": "a-secure-test-password",
            "display_name": "创作者",
        },
    )


@pytest.mark.asyncio
async def test_registration_creates_one_atomic_personal_workspace(
    client: httpx.AsyncClient,
    session_factory: async_sessionmaker[AsyncSession],
) -> None:
    first, second = await asyncio.gather(_register(client), _register(client))

    assert sorted((first.status_code, second.status_code)) == [201, 409]
    success = first if first.status_code == 201 else second
    payload = success.json()["data"]
    assert payload["user"]["email"] == "creator@example.com"
    assert payload["user"]["id"]
    assert payload["workspace"]["role"] == "owner"
    assert payload["access_token"]
    assert payload["token_type"] == "bearer"

    async with session_factory() as session:
        assert await session.scalar(select(func.count()).select_from(UserAccount)) == 1
        assert await session.scalar(select(func.count()).select_from(Workspace)) == 1
        assert await session.scalar(select(func.count()).select_from(Membership)) == 1
        user = await session.scalar(select(UserAccount))
        assert user is not None
        assert user.id.version == 7
        assert user.password_hash != "a-secure-test-password"


@pytest.mark.asyncio
async def test_login_does_not_reveal_whether_an_account_exists(
    client: httpx.AsyncClient,
) -> None:
    await _register(client)
    headers = {"x-request-id": "same-request"}
    wrong_password = await client.post(
        "/api/v1/auth/login",
        headers=headers,
        json={"email": "creator@example.com", "password": "wrong-password-value"},
    )
    missing_account = await client.post(
        "/api/v1/auth/login",
        headers=headers,
        json={"email": "missing@example.com", "password": "wrong-password-value"},
    )

    assert wrong_password.status_code == missing_account.status_code == 401
    assert wrong_password.json() == missing_account.json()
    assert "wrong-password-value" not in wrong_password.text


@pytest.mark.asyncio
async def test_logout_and_password_change_revoke_previous_tokens(
    client: httpx.AsyncClient,
) -> None:
    registered = await _register(client)
    first_token = registered.json()["data"]["access_token"]
    first_headers = {"authorization": f"Bearer {first_token}"}
    assert (await client.get("/api/v1/me", headers=first_headers)).status_code == 200

    assert (await client.post("/api/v1/auth/logout", headers=first_headers)).status_code == 200
    assert (await client.get("/api/v1/me", headers=first_headers)).status_code == 401

    logged_in = await client.post(
        "/api/v1/auth/login",
        json={"email": "creator@example.com", "password": "a-secure-test-password"},
    )
    second_token = logged_in.json()["data"]["access_token"]
    second_headers = {"authorization": f"Bearer {second_token}"}
    changed = await client.post(
        "/api/v1/auth/change-password",
        headers=second_headers,
        json={
            "current_password": "a-secure-test-password",
            "new_password": "a-new-secure-test-password",
        },
    )
    assert changed.status_code == 200
    assert (await client.get("/api/v1/me", headers=second_headers)).status_code == 401

    old_login = await client.post(
        "/api/v1/auth/login",
        json={"email": "creator@example.com", "password": "a-secure-test-password"},
    )
    new_login = await client.post(
        "/api/v1/auth/login",
        json={"email": "creator@example.com", "password": "a-new-secure-test-password"},
    )
    assert old_login.status_code == 401
    assert new_login.status_code == 200


@pytest.mark.asyncio
async def test_profile_and_workspace_lifecycle_are_versioned_and_isolated(
    client: httpx.AsyncClient,
) -> None:
    registered = await _register(client)
    token = registered.json()["data"]["access_token"]
    headers = {"authorization": f"Bearer {token}"}

    rejected = await client.patch(
        "/api/v1/me",
        headers=headers,
        json={"email": "takeover@example.com"},
    )
    assert rejected.status_code == 422

    updated_profile = await client.patch(
        "/api/v1/me",
        headers=headers,
        json={"display_name": "新名称", "avatar_url": "https://example.com/avatar.png"},
    )
    assert updated_profile.status_code == 200
    assert updated_profile.json()["data"]["user"]["display_name"] == "新名称"

    created = await client.post(
        "/api/v1/workspaces",
        headers=headers,
        json={"name": "第二工作空间"},
    )
    assert created.status_code == 201
    workspace = created.json()["data"]
    workspace_id = workspace["id"]
    assert workspace["revision"] == 1

    renamed = await client.patch(
        f"/api/v1/workspaces/{workspace_id}",
        headers=headers,
        json={"name": "正式空间", "expected_revision": 1},
    )
    assert renamed.status_code == 200
    assert renamed.json()["data"]["revision"] == 2

    stale = await client.patch(
        f"/api/v1/workspaces/{workspace_id}",
        headers=headers,
        json={"name": "覆盖空间", "expected_revision": 1},
    )
    assert stale.status_code == 409
    assert stale.json()["error"]["code"] == "version_conflict"

    archived = await client.post(
        f"/api/v1/workspaces/{workspace_id}/archive",
        headers=headers,
        json={"expected_revision": 2},
    )
    assert archived.status_code == 200
    assert archived.json()["data"]["status"] == "archived"
    assert archived.json()["data"]["revision"] == 3

    restored = await client.post(
        f"/api/v1/workspaces/{workspace_id}/restore",
        headers=headers,
        json={"expected_revision": 3},
    )
    assert restored.status_code == 200
    assert restored.json()["data"]["status"] == "active"
    assert restored.json()["data"]["revision"] == 4

    listed = await client.get("/api/v1/workspaces", headers=headers)
    assert listed.status_code == 200
    assert len(listed.json()["data"]) == 2

    other_registration = await client.post(
        "/api/v1/auth/register",
        json={
            "email": "other@example.com",
            "password": "another-secure-password",
            "display_name": "其他用户",
        },
    )
    other_token = other_registration.json()["data"]["access_token"]
    hidden = await client.get(
        f"/api/v1/workspaces/{workspace_id}",
        headers={"authorization": f"Bearer {other_token}"},
    )
    assert hidden.status_code == 404


@pytest.mark.asyncio
async def test_account_deactivation_preserves_history_and_revokes_access(
    client: httpx.AsyncClient,
    session_factory: async_sessionmaker[AsyncSession],
) -> None:
    registered = await _register(client)
    token = registered.json()["data"]["access_token"]
    headers = {"authorization": f"Bearer {token}"}

    deactivated = await client.post(
        "/api/v1/me/deactivate",
        headers=headers,
        json={"confirmation": "DEACTIVATE"},
    )
    assert deactivated.status_code == 200
    assert (await client.get("/api/v1/me", headers=headers)).status_code == 401

    login = await client.post(
        "/api/v1/auth/login",
        json={"email": "creator@example.com", "password": "a-secure-test-password"},
    )
    assert login.status_code == 401

    async with session_factory() as session:
        user = await session.scalar(select(UserAccount))
        assert user is not None and user.status == "deactivated"
        assert await session.scalar(select(func.count()).select_from(Workspace)) == 1
        assert await session.scalar(select(func.count()).select_from(Membership)) == 1
