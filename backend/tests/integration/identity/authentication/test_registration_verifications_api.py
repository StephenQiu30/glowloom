import httpx
import pytest
from sqlalchemy import func, select
from sqlalchemy.ext.asyncio import AsyncSession, async_sessionmaker

from app.modules.identity.models import Membership, UserAccount, Workspace
from tests.support.identity_builders import (
    register_identity_response,
    request_registration_ticket,
)
from tests.support.registration_verification import (
    MemoryRegistrationVerificationStore,
    RecordingRegistrationMailer,
)


@pytest.mark.asyncio
async def test_registration_requires_confirmed_email_and_consumes_ticket_once(
    client: httpx.AsyncClient,
    session_factory: async_sessionmaker[AsyncSession],
) -> None:
    bypass = await client.post(
        "/api/v1/auth/register",
        json={
            "email": "creator@example.com",
            "password": "a-secure-test-password",
            "display_name": "创作者",
        },
    )
    assert bypass.status_code == 422

    requested = await client.post(
        "/api/v1/auth/registration-verifications",
        json={"email": "Creator@Example.com"},
    )
    assert requested.status_code == 202
    assert requested.json()["data"] == {
        "accepted": True,
        "email_sent": True,
        "retry_after_seconds": 60,
    }

    wrong = await client.post(
        "/api/v1/auth/registration-verifications/confirm",
        json={"email": "creator@example.com", "code": "000000"},
    )
    assert wrong.status_code == 422
    assert wrong.json()["error"]["code"] == "invalid_verification_code"
    assert wrong.json()["error"]["details"] == {"remaining_attempts": 4}

    confirmed = await client.post(
        "/api/v1/auth/registration-verifications/confirm",
        json={"email": "creator@example.com", "code": "123456"},
    )
    assert confirmed.status_code == 200
    ticket = confirmed.json()["data"]["registration_ticket"]
    assert len(ticket) >= 43
    assert confirmed.json()["data"]["expires_in"] == 600

    payload = {
        "registration_ticket": ticket,
        "password": "a-secure-test-password",
        "display_name": "创作者",
    }
    registered = await client.post("/api/v1/auth/register", json=payload)
    reused = await client.post("/api/v1/auth/register", json=payload)
    assert registered.status_code == 201
    assert registered.json()["data"]["user"]["email"] == "creator@example.com"
    assert reused.status_code == 410
    assert reused.json()["error"]["code"] == "verification_expired"

    async with session_factory() as session:
        assert await session.scalar(select(func.count()).select_from(UserAccount)) == 1
        assert await session.scalar(select(func.count()).select_from(Workspace)) == 1
        assert await session.scalar(select(func.count()).select_from(Membership)) == 1


@pytest.mark.asyncio
async def test_registered_email_reports_no_send_without_another_code(
    client: httpx.AsyncClient,
    registration_mailer: RecordingRegistrationMailer,
) -> None:
    registered = await register_identity_response(client)
    assert registered.status_code == 201
    sent_before = len(registration_mailer.messages)

    response = await client.post(
        "/api/v1/auth/registration-verifications",
        json={"email": "creator@example.com"},
    )

    assert response.status_code == 202
    assert response.json()["data"] == {
        "accepted": True,
        "email_sent": False,
        "retry_after_seconds": 60,
    }
    assert len(registration_mailer.messages) == sent_before


@pytest.mark.asyncio
async def test_invalid_profile_does_not_consume_registration_ticket(
    client: httpx.AsyncClient,
) -> None:
    ticket = await request_registration_ticket(client, email="profile@example.com")
    invalid = await client.post(
        "/api/v1/auth/register",
        json={
            "registration_ticket": ticket,
            "password": "too-short",
            "display_name": "创作者",
        },
    )
    valid = await client.post(
        "/api/v1/auth/register",
        json={
            "registration_ticket": ticket,
            "password": "a-secure-test-password",
            "display_name": "创作者",
        },
    )

    assert invalid.status_code == 422
    assert valid.status_code == 201


@pytest.mark.asyncio
async def test_registration_dependencies_fail_closed_but_login_stays_available(
    client: httpx.AsyncClient,
    registration_store: MemoryRegistrationVerificationStore,
    registration_mailer: RecordingRegistrationMailer,
) -> None:
    registered = await register_identity_response(client)
    assert registered.status_code == 201

    registration_store.unavailable = True
    unavailable_store = await client.post(
        "/api/v1/auth/registration-verifications",
        json={"email": "new@example.com"},
    )
    assert unavailable_store.status_code == 503
    assert unavailable_store.json()["error"]["code"] == "dependency_unavailable"

    registration_store.unavailable = False
    registration_mailer.unavailable = True
    unavailable_mail = await client.post(
        "/api/v1/auth/registration-verifications",
        json={"email": "new@example.com"},
    )
    assert unavailable_mail.status_code == 503
    assert unavailable_mail.json()["error"]["code"] == "dependency_unavailable"
    assert "test registration mailer" not in unavailable_mail.text

    registration_store.unavailable = True
    login = await client.post(
        "/api/v1/auth/login",
        json={
            "email": "creator@example.com",
            "password": "a-secure-test-password",
        },
    )
    assert login.status_code == 200
