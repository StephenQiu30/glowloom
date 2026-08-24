import pytest
from pydantic import SecretStr

from app.core.config import Settings
from app.runtime.local_development import resolve_local_database_url


def test_local_runtime_uses_an_isolated_database_on_the_same_postgres_service() -> None:
    settings = Settings(
        environment="development",
        database_url="postgresql+asyncpg://developer:secret@127.0.0.1:5432/lanverse",
    )

    target = resolve_local_database_url(settings, requested_name=None)

    assert target.database == "lanverse_development"
    assert target.host == "127.0.0.1"
    assert target.port == 5432
    assert target.username == "developer"


def test_local_runtime_accepts_an_explicit_safe_database_name() -> None:
    settings = Settings(
        environment="development",
        database_url="postgresql+asyncpg://postgres@127.0.0.1/lanverse",
    )

    target = resolve_local_database_url(settings, requested_name="lanverse_feature_test")

    assert target.database == "lanverse_feature_test"


def test_local_runtime_rejects_production_and_unsafe_database_names() -> None:
    production = Settings(
        environment="production",
        database_url="postgresql+asyncpg://postgres@127.0.0.1/lanverse",
        jwt_secret_key=SecretStr("production-jwt-secret-with-at-least-32-bytes"),
        email_verification_hmac_secret=SecretStr(
            "production-registration-secret-with-at-least-32-bytes"
        ),
    )

    with pytest.raises(ValueError, match="development"):
        resolve_local_database_url(production, requested_name=None)
    with pytest.raises(ValueError, match="database name"):
        resolve_local_database_url(
            Settings(environment="development"),
            requested_name="lanverse;drop database postgres",
        )
