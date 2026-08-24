from unittest.mock import AsyncMock, Mock

import pytest
from pydantic import SecretStr

from app.core.config import Settings
from app.runtime.commands import database as database_command


@pytest.mark.asyncio
async def test_database_prepare_only_validates_compatible_schema(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    engine = Mock()
    engine.dispose = AsyncMock()
    assert_database_schema = AsyncMock()
    monkeypatch.setattr(database_command, "engine", engine)
    monkeypatch.setattr(database_command, "assert_database_schema", assert_database_schema)

    await database_command.prepare_database(Settings(environment="test"))

    assert_database_schema.assert_awaited_once_with(engine)
    engine.dispose.assert_awaited_once_with()


@pytest.mark.asyncio
async def test_production_database_prepare_only_validates_schema(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    engine = Mock()
    engine.dispose = AsyncMock()
    assert_database_schema = AsyncMock()
    monkeypatch.setattr(database_command, "engine", engine)
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
    engine.dispose.assert_awaited_once_with()


@pytest.mark.asyncio
async def test_baseline_adoption_is_an_explicit_command(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    engine = Mock()
    engine.dispose = AsyncMock()
    adopt_database_baseline = AsyncMock()
    monkeypatch.setattr(database_command, "engine", engine)
    monkeypatch.setattr(
        database_command,
        "adopt_database_baseline",
        adopt_database_baseline,
    )

    await database_command.adopt_baseline()

    adopt_database_baseline.assert_awaited_once_with(engine)
    engine.dispose.assert_awaited_once_with()
