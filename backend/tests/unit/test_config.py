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
        "ARK_API_KEY",
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


def test_workspace_cache_ttl_and_jitter_are_bounded() -> None:
    settings = Settings()
    assert 1 <= settings.workspace_cache_ttl_seconds <= 3600
    assert 0 <= settings.cache_ttl_jitter_seconds < settings.workspace_cache_ttl_seconds

    with pytest.raises(ValueError, match="must be lower"):
        Settings.model_validate(
            {
                "workspace_cache_ttl_seconds": 10,
                "cache_ttl_jitter_seconds": 10,
            }
        )


def test_generation_high_cost_guard_limits_are_bounded_and_ordered() -> None:
    settings = Settings()
    assert settings.generation_high_cost_window_seconds == 60
    assert settings.generation_high_cost_workspace_limit == 3
    assert settings.generation_high_cost_global_limit == 20
    assert settings.generation_high_cost_idempotency_ttl_seconds == 900

    with pytest.raises(ValueError, match="GLOBAL_LIMIT"):
        Settings.model_validate(
            {
                "generation_high_cost_workspace_limit": 4,
                "generation_high_cost_global_limit": 3,
            }
        )
    with pytest.raises(ValueError, match="IDEMPOTENCY_TTL_SECONDS"):
        Settings.model_validate(
            {
                "generation_high_cost_window_seconds": 61,
                "generation_high_cost_idempotency_ttl_seconds": 60,
            }
        )


def test_media_cleanup_schedule_and_batch_are_bounded() -> None:
    settings = Settings()

    assert 60 <= settings.media_cleanup_interval_seconds <= 86400
    assert 1 <= settings.media_cleanup_batch_size <= 500

    with pytest.raises(ValueError):
        Settings.model_validate({"media_cleanup_interval_seconds": 59})
    with pytest.raises(ValueError):
        Settings.model_validate({"media_cleanup_batch_size": 501})


def test_media_location_rollback_window_is_bounded() -> None:
    settings = Settings()

    assert settings.media_location_rollback_seconds == 86400
    with pytest.raises(ValueError):
        Settings.model_validate({"media_location_rollback_seconds": 59})
    with pytest.raises(ValueError):
        Settings.model_validate({"media_location_rollback_seconds": 604801})


def test_storage_concurrency_and_operation_timeout_are_bounded() -> None:
    settings = Settings()

    assert 1 <= settings.storage_thread_limit <= 32
    assert 0.1 <= settings.storage_operation_timeout_seconds <= 30
    with pytest.raises(ValueError):
        Settings.model_validate({"storage_thread_limit": 0})
    with pytest.raises(ValueError):
        Settings.model_validate({"storage_operation_timeout_seconds": 0})


def test_server_bind_address_is_explicit_and_validated() -> None:
    settings = Settings.model_validate(
        {"api_host": "127.0.0.1", "api_port": 8001}
    )

    assert settings.api_host == "127.0.0.1"
    assert settings.api_port == 8001

    with pytest.raises(ValueError):
        Settings.model_validate({"api_port": 0})


def test_provider_keys_are_optional_and_secret() -> None:
    assert Settings.model_validate({"deepseek_api_key": None}).deepseek_api_key is None
    assert Settings.model_validate({"deepseek_api_key": ""}).deepseek_api_key is None
    assert Settings.model_validate({"ark_api_key": None}).ark_api_key is None
    assert Settings.model_validate({"ark_api_key": ""}).ark_api_key is None

    configured = Settings.model_validate(
        {
            "deepseek_api_key": "test-deepseek-key",
            "ark_api_key": "test-ark-key",
        }
    )

    assert isinstance(configured.deepseek_api_key, SecretStr)
    assert str(configured.deepseek_api_key) == "**********"
    assert isinstance(configured.ark_api_key, SecretStr)
    assert str(configured.ark_api_key) == "**********"
