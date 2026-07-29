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
from app.modules.identity.models import Membership, UserAccount, Workspace

TEST_DATABASE_URL = validate_test_database_url(
    "postgresql+asyncpg://postgres@127.0.0.1:5432/lanverse_test",
    "postgresql+asyncpg://postgres@127.0.0.1:5432/lanverse",
)
TEST_JWT_SECRET = SecretStr("identity-test-secret-with-at-least-32-bytes")


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
            jwt_secret_key=TEST_JWT_SECRET,
        )
    )
    app.dependency_overrides[get_async_session] = _test_session
    transport = httpx.ASGITransport(app=app)
    async with httpx.AsyncClient(transport=transport, base_url="http://test") as test_client:
        yield test_client


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
