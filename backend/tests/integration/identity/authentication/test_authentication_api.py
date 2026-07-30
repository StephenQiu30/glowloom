import asyncio

import httpx
import pytest
from sqlalchemy import func, select
from sqlalchemy.ext.asyncio import AsyncSession, async_sessionmaker

from app.modules.identity.models import Membership, UserAccount, Workspace
from tests.support.identity_builders import register_identity_response


@pytest.mark.asyncio
async def test_registration_creates_one_atomic_personal_workspace(
    client: httpx.AsyncClient,
    session_factory: async_sessionmaker[AsyncSession],
) -> None:
    first, second = await asyncio.gather(
        register_identity_response(client), register_identity_response(client)
    )

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
    await register_identity_response(client)
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
    registered = await register_identity_response(client)
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
async def test_account_deactivation_preserves_history_and_revokes_access(
    client: httpx.AsyncClient,
    session_factory: async_sessionmaker[AsyncSession],
) -> None:
    registered = await register_identity_response(client)
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
