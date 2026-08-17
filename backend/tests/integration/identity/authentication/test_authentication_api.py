import asyncio

import httpx
import pytest
from sqlalchemy import func, select
from sqlalchemy.ext.asyncio import AsyncSession, async_sessionmaker
from uuid6 import uuid7

from app.modules.governance.audit.models import AuditEvent
from app.modules.identity.models import Membership, UserAccount, Workspace
from tests.support.identity_builders import (
    register_identity_response,
    request_registration_ticket,
)


@pytest.mark.asyncio
async def test_registration_creates_one_atomic_personal_workspace(
    client: httpx.AsyncClient,
    session_factory: async_sessionmaker[AsyncSession],
) -> None:
    ticket = await request_registration_ticket(client, email="Creator@Example.com")
    registration_payload = {
        "registration_ticket": ticket,
        "password": "a-secure-test-password",
        "display_name": "创作者",
    }
    first, second = await asyncio.gather(
        client.post("/api/v1/auth/register", json=registration_payload),
        client.post("/api/v1/auth/register", json=registration_payload),
    )

    assert sorted((first.status_code, second.status_code)) == [201, 410]
    success = first if first.status_code == 201 else second
    payload = success.json()["data"]
    assert payload["user"]["email"] == "creator@example.com"
    assert payload["user"]["id"]
    assert payload["workspace"]["role"] == "owner"
    assert payload["access_token"]
    assert payload["token_type"] == "bearer"

    audit = await client.get(
        "/api/v1/audit-events",
        headers={"authorization": f"Bearer {payload['access_token']}"},
        params={
            "workspace_id": payload["workspace"]["id"],
            "target_type": "user_account",
            "target_id": payload["user"]["id"],
        },
    )
    assert audit.status_code == 200
    assert audit.json()["data"]["total"] == 1
    registered_event = audit.json()["data"]["items"][0]
    assert registered_event["action"] == "identity.registered"
    assert registered_event["actor_id"] == payload["user"]["id"]
    assert registered_event["result"] == "succeeded"
    assert registered_event["trace_id"]
    assert registered_event["metadata"] == {
        "token_version": 1,
        "workspace_revision": 1,
    }
    assert "email" not in str(registered_event).lower()
    assert "password" not in str(registered_event).lower()

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
    registered = await register_identity_response(client)
    headers = {"x-request-id": str(uuid7())}
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

    audit = await client.get(
        "/api/v1/audit-events",
        headers={
            "authorization": f"Bearer {registered.json()['data']['access_token']}"
        },
        params={
            "workspace_id": registered.json()["data"]["workspace"]["id"],
            "action": "identity.login_succeeded",
        },
    )
    assert audit.status_code == 200
    assert audit.json()["data"]["total"] == 0


@pytest.mark.asyncio
async def test_refresh_rotates_persistent_session_and_rejects_replay(
    client: httpx.AsyncClient,
) -> None:
    registered = await register_identity_response(client)
    assert registered.status_code == 201
    first_token = registered.json()["data"]["access_token"]
    first_refresh = registered.cookies.get("lanverse_refresh_token")
    assert first_refresh

    refreshed = await client.post("/api/v1/auth/refresh")
    assert refreshed.status_code == 200
    second_token = refreshed.json()["data"]["access_token"]
    second_refresh = refreshed.cookies.get("lanverse_refresh_token")
    assert second_token
    assert second_token != first_token
    assert second_refresh and second_refresh != first_refresh

    replay = await client.post(
        "/api/v1/auth/refresh",
        headers={"cookie": f"lanverse_refresh_token={first_refresh}"},
    )
    assert replay.status_code == 401
    assert (await client.get(
        "/api/v1/me",
        headers={"authorization": f"Bearer {second_token}"},
    )).status_code == 200


@pytest.mark.asyncio
async def test_logout_revokes_persistent_session(
    client: httpx.AsyncClient,
) -> None:
    registered = await register_identity_response(client)
    token = registered.json()["data"]["access_token"]
    assert registered.cookies.get("lanverse_refresh_token")

    logged_out = await client.post(
        "/api/v1/auth/logout",
        headers={"authorization": f"Bearer {token}"},
    )
    assert logged_out.status_code == 200
    assert (await client.post("/api/v1/auth/refresh")).status_code == 401


@pytest.mark.asyncio
async def test_logout_and_password_change_revoke_previous_tokens(
    client: httpx.AsyncClient,
    session_factory: async_sessionmaker[AsyncSession],
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

    audit = await client.get(
        "/api/v1/audit-events",
        headers={
            "authorization": f"Bearer {new_login.json()['data']['access_token']}"
        },
        params={
            "workspace_id": registered.json()["data"]["workspace"]["id"],
            "target_type": "user_account",
            "target_id": registered.json()["data"]["user"]["id"],
        },
    )
    assert audit.status_code == 200
    actions = [item["action"] for item in audit.json()["data"]["items"]]
    assert actions == [
        "identity.login_succeeded",
        "identity.password_changed",
        "identity.login_succeeded",
        "identity.logged_out",
        "identity.registered",
    ]
    assert all(
        set(item["metadata"])
        <= {"token_version", "previous_token_version", "workspace_revision"}
        for item in audit.json()["data"]["items"]
    )

    async with session_factory() as session:
        events = list(
            await session.scalars(
                select(AuditEvent).order_by(
                    AuditEvent.occurred_at.asc(), AuditEvent.id.asc()
                )
            )
        )
        assert len(events) == 5


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
        events = list(
            await session.scalars(
                select(AuditEvent).order_by(
                    AuditEvent.occurred_at.asc(), AuditEvent.id.asc()
                )
            )
        )
        assert [event.action for event in events] == [
            "identity.registered",
            "identity.account_deactivated",
        ]
        deactivated_event = events[-1]
        assert str(deactivated_event.workspace_id) == registered.json()["data"]["workspace"]["id"]
        assert deactivated_event.actor_id == user.id
        assert deactivated_event.target_id == user.id
        assert deactivated_event.event_metadata == {
            "previous_status": "active",
            "status": "deactivated",
            "previous_token_version": 1,
            "token_version": 2,
        }
