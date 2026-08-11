from collections.abc import AsyncIterator

import httpx
import pytest
from fastapi import FastAPI
from pydantic import SecretStr
from sqlalchemy.ext.asyncio import AsyncSession, async_sessionmaker

from app.core.config import Settings
from app.core.database import (
    Base,
    create_engine,
    get_async_session,
    validate_test_database_url,
)
from app.main import create_app
from app.modules.caching.contracts import (
    CacheKey,
    CacheNamespace,
    CachePort,
    CacheUnavailableError,
    HighCostGuardRequest,
    HighCostGuardResult,
)
from app.modules.caching.dependencies import get_cache_port, get_high_cost_guard
from tests.support.registration_verification import (
    MemoryRegistrationVerificationStore,
    RecordingRegistrationMailer,
)

TEST_DATABASE_URL = validate_test_database_url(
    "postgresql+asyncpg://postgres@127.0.0.1:5432/lanverse_test",
    "postgresql+asyncpg://postgres@127.0.0.1:5432/lanverse",
)


class UnavailableTestCache:
    async def get(self, key: CacheKey) -> bytes | None:
        raise CacheUnavailableError("test cache is disabled")

    async def set(
        self,
        key: CacheKey,
        value: bytes,
        *,
        ttl_seconds: int,
        jitter_seconds: int,
    ) -> None:
        raise CacheUnavailableError("test cache is disabled")

    async def delete(self, key: CacheKey) -> None:
        raise CacheUnavailableError("test cache is disabled")

    async def set_revision(
        self,
        key: CacheKey,
        revision: int,
        *,
        ttl_seconds: int,
        jitter_seconds: int,
    ) -> bool:
        raise CacheUnavailableError("test cache is disabled")

    async def invalidate_namespace(self, namespace: CacheNamespace) -> int:
        raise CacheUnavailableError("test cache is disabled")

    async def close(self) -> None:
        return None


class AllowingTestHighCostGuard:
    def __init__(self) -> None:
        self.calls: list[HighCostGuardRequest] = []

    async def authorize_high_cost(
        self,
        request: HighCostGuardRequest,
    ) -> HighCostGuardResult:
        self.calls.append(request)
        return HighCostGuardResult(allowed=True, outcome="allowed", retry_after_seconds=None)


@pytest.fixture
def test_settings() -> Settings:
    return Settings(
        environment="test",
        database_url=TEST_DATABASE_URL,
        jwt_secret_key=SecretStr("integration-test-secret-with-at-least-32-bytes"),
        email_verification_hmac_secret=SecretStr(
            "integration-registration-secret-with-at-least-32-bytes"
        ),
    )


@pytest.fixture
def cache_port() -> CachePort:
    return UnavailableTestCache()


@pytest.fixture
def high_cost_guard() -> AllowingTestHighCostGuard:
    return AllowingTestHighCostGuard()


@pytest.fixture
def registration_store() -> MemoryRegistrationVerificationStore:
    return MemoryRegistrationVerificationStore()


@pytest.fixture
def registration_mailer() -> RecordingRegistrationMailer:
    return RecordingRegistrationMailer()


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
def app(
    session_factory: async_sessionmaker[AsyncSession],
    test_settings: Settings,
    cache_port: CachePort,
    high_cost_guard: AllowingTestHighCostGuard,
    registration_store: MemoryRegistrationVerificationStore,
    registration_mailer: RecordingRegistrationMailer,
) -> FastAPI:
    async def test_session() -> AsyncIterator[AsyncSession]:
        async with session_factory() as session:
            yield session

    app = create_app(test_settings)
    app.dependency_overrides[get_async_session] = test_session
    app.dependency_overrides[get_cache_port] = lambda: cache_port
    app.dependency_overrides[get_high_cost_guard] = lambda: high_cost_guard
    app.state.registration_verification_store = registration_store
    app.state.registration_mailer = registration_mailer
    app.state.registration_code_generator = lambda: "123456"
    return app


@pytest.fixture
async def client(app: FastAPI) -> AsyncIterator[httpx.AsyncClient]:
    async with httpx.AsyncClient(
        transport=httpx.ASGITransport(app=app),
        base_url="http://test",
    ) as test_client:
        yield test_client
