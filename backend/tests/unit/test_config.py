from pathlib import Path

import pytest
from pydantic import SecretStr

from app.core.config import REPOSITORY_ENV_FILE, Settings
from app.core.database import validate_test_database_url

ROOT = Path(__file__).resolve().parents[3]


def _read_environment_example(filename: str) -> dict[str, str]:
    values: dict[str, str] = {}
    for line in (ROOT / filename).read_text(encoding="utf-8").splitlines():
        if line and not line.startswith("#") and "=" in line:
            key, _, value = line.partition("=")
            values[key] = value
    return values


def test_settings_load_only_the_repository_environment_file() -> None:
    assert Settings.model_config.get("env_file") == REPOSITORY_ENV_FILE
    assert REPOSITORY_ENV_FILE == ROOT / ".env"


def test_repository_environment_example_covers_all_backend_settings() -> None:
    environment_keys = set(_read_environment_example(".env.example"))

    assert {name.upper() for name in Settings.model_fields} <= environment_keys


def test_production_environment_example_is_fail_closed() -> None:
    development_values = _read_environment_example(".env.example")
    production_values = _read_environment_example(".env.production.example")

    assert production_values.keys() == development_values.keys()
    assert production_values["ENVIRONMENT"] == "production"
    assert production_values["NEXT_PUBLIC_API_BASE_URL"].startswith("https://")
    assert {
        "DATABASE_URL",
        "TEST_DATABASE_URL",
        "POSTGRES_DB",
        "POSTGRES_USER",
        "POSTGRES_PASSWORD",
        "POSTGRES_PORT",
    } <= production_values.keys()
    for secret in (
        "DATABASE_URL",
        "TEST_DATABASE_URL",
        "POSTGRES_PASSWORD",
        "RABBITMQ_URL",
        "RABBITMQ_DEFAULT_PASS",
        "MINIO_ACCESS_KEY",
        "MINIO_SECRET_KEY",
        "JWT_SECRET_KEY",
        "DEEPSEEK_API_KEY",
    ):
        assert production_values[secret] == ""


def test_test_database_must_be_explicit() -> None:
    with pytest.raises(ValueError, match="required"):
        validate_test_database_url(None, "postgresql+asyncpg://postgres/lanverse")


def test_test_database_name_must_end_with_test() -> None:
    with pytest.raises(ValueError, match="end with _test"):
        validate_test_database_url(
            "postgresql+asyncpg://postgres/lanverse",
            "postgresql+asyncpg://postgres/application",
        )


def test_test_database_must_not_equal_application_database() -> None:
    url = "postgresql+asyncpg://postgres/lanverse_test"
    with pytest.raises(ValueError, match="must not equal"):
        validate_test_database_url(url, url)


def test_outbox_resource_limits_are_bounded() -> None:
    settings = Settings()
    assert 1 <= settings.outbox_batch_size <= 100
    assert 5 <= settings.outbox_claim_seconds <= 3600
    assert 0.1 <= settings.outbox_poll_seconds <= 60


def test_server_bind_address_is_explicit_and_validated() -> None:
    settings = Settings.model_validate(
        {"api_host": "127.0.0.1", "api_port": 8001}
    )

    assert settings.api_host == "127.0.0.1"
    assert settings.api_port == 8001

    with pytest.raises(ValueError):
        Settings.model_validate({"api_port": 0})


def test_deepseek_key_is_optional_and_secret() -> None:
    assert Settings.model_validate({}).deepseek_api_key is None
    assert Settings.model_validate({"deepseek_api_key": ""}).deepseek_api_key is None

    configured = Settings.model_validate({"deepseek_api_key": "test-deepseek-key"})

    assert isinstance(configured.deepseek_api_key, SecretStr)
    assert str(configured.deepseek_api_key) == "**********"
