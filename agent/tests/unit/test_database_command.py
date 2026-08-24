from unittest.mock import AsyncMock, Mock

import pytest
from pydantic import SecretStr

from app.core.config import Settings
from app.runtime.commands import database as database_command


@pytest.mark.asyncio
async def test_development_database_prepare_initializes_schema(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    engine = Mock()
    engine.dispose = AsyncMock()
    initialize_database = AsyncMock()
    assert_database_schema = AsyncMock()
    monkeypatch.setattr(database_command, "engine", engine)
    monkeypatch.setattr(database_command, "initialize_database", initialize_database)
    monkeypatch.setattr(database_command, "assert_database_schema", assert_database_schema)

    await database_command.prepare_database(Settings(environment="test"))

    initialize_database.assert_awaited_once_with(engine)
    assert_database_schema.assert_not_awaited()
    engine.dispose.assert_awaited_once_with()


@pytest.mark.asyncio
async def test_production_database_prepare_only_validates_schema(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    engine = Mock()
    engine.dispose = AsyncMock()
    initialize_database = AsyncMock()
    assert_database_schema = AsyncMock()
    monkeypatch.setattr(database_command, "engine", engine)
    monkeypatch.setattr(database_command, "initialize_database", initialize_database)
    monkeypatch.setattr(database_command, "assert_database_schema", assert_database_schema)
    settings = Settings(
        environment="production",
        jwt_secret_key=SecretStr("production-jwt-secret-with-at-least-32-bytes"),
        email_verification_hmac_secret=SecretStr(
            "production-registration-secret-with-at-least-32-bytes"
        ),
    )

    await database_command.prepare_database(settings)

    assert_database_schema.assert_awaited_once_with(engine)
    initialize_database.assert_not_awaited()
    engine.dispose.assert_awaited_once_with()
