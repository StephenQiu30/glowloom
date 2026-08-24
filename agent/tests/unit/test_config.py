import json
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


def test_application_ports_and_development_cors_follow_the_fixed_contract() -> None:
    supported_origins = {
        "http://localhost:8123",
        "http://127.0.0.1:8123",
    }
    default_origins = set(Settings.model_fields["cors_origins"].default)
    example_origins = set(json.loads(_read_environment_example(".env.example")["CORS_ORIGINS"]))
    development_origins = set(
        Settings.model_validate(
            {
                "environment": "development",
                "cors_origins": ["http://localhost:8123"],
            }
        ).cors_origins
    )

    assert Settings.model_fields["api_port"].default == 8686
    assert _read_environment_example(".env.example")["API_PORT"] == "8686"
    assert default_origins == supported_origins
    assert example_origins == supported_origins
    assert development_origins == supported_origins


def test_production_cors_does_not_add_development_origins() -> None:
    settings = Settings.model_validate(
        {
            "environment": "production",
            "cors_origins": ["https://app.example.com"],
            "jwt_secret_key": "production-test-secret-with-at-least-32-bytes",
            "email_verification_hmac_secret": (
                "production-registration-secret-with-at-least-32-bytes"
            ),
        }
    )

    assert settings.cors_origins == ["https://app.example.com"]


def test_production_environment_example_is_fail_closed() -> None:
    development_values = _read_environment_example(".env.example")
    production_values = _read_environment_example(".env.production.example")

    assert production_values.keys() == development_values.keys()
    assert production_values["ENVIRONMENT"] == "production"
    assert production_values["NEXT_PUBLIC_API_BASE_URL"].startswith("https://")
    assert {"DATABASE_URL", "TEST_DATABASE_URL"} <= production_values.keys()
    for secret in (
        "DATABASE_URL",
        "TEST_DATABASE_URL",
        "KAFKA_BOOTSTRAP_SERVERS",
        "MINIO_ACCESS_KEY",
        "MINIO_SECRET_KEY",
        "JWT_SECRET_KEY",
        "SMTP_PASSWORD",
        "EMAIL_VERIFICATION_HMAC_SECRET",
        "ARK_API_KEY",
        "PROVIDER_CREDENTIAL_MASTER_KEY",
        "PROVIDER_CREDENTIAL_FINGERPRINT_KEY",
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
    settings = Settings.model_validate({"api_host": "127.0.0.1", "api_port": 8001})

    assert settings.api_host == "127.0.0.1"
    assert settings.api_port == 8001

    with pytest.raises(ValueError):
        Settings.model_validate({"api_port": 0})


def test_provider_keys_are_optional_and_secret() -> None:
    assert Settings.model_validate({"ark_api_key": None}).ark_api_key is None
    assert Settings.model_validate({"ark_api_key": ""}).ark_api_key is None

    configured = Settings.model_validate({"ark_api_key": "test-ark-key"})

    assert isinstance(configured.ark_api_key, SecretStr)
    assert str(configured.ark_api_key) == "**********"


def test_local_codex_settings_are_available_without_provider_credentials() -> None:
    defaults = Settings.model_validate({"codex_cli_path": "", "codex_model": ""})
    settings = Settings.model_validate(
        {
            "codex_cli_path": "/usr/local/bin/codex",
            "codex_model": "gpt-5-codex",
            "codex_max_concurrency": 3,
        }
    )

    assert defaults.codex_cli_path is None
    assert defaults.codex_model is None
    assert settings.codex_cli_path == "/usr/local/bin/codex"
    assert settings.codex_model == "gpt-5-codex"
    assert settings.codex_max_concurrency == 3


def test_provider_credential_encryption_keys_are_optional_independent_secrets() -> None:
    settings = Settings()

    assert settings.provider_credential_key_id == "local-provider-v1"
    assert settings.provider_credential_master_key is None
    assert settings.provider_credential_fingerprint_key is None

    configured = Settings.model_validate(
        {
            "provider_credential_master_key": "test-master-key-material",
            "provider_credential_fingerprint_key": "test-fingerprint-key-material",
        }
    )
    assert str(configured.provider_credential_master_key) == "**********"
    assert str(configured.provider_credential_fingerprint_key) == "**********"

    with pytest.raises(ValueError, match="must not reuse"):
        Settings.model_validate(
            {
                "provider_credential_master_key": "same-provider-key-material",
                "provider_credential_fingerprint_key": "same-provider-key-material",
            }
        )


def test_registration_email_settings_are_bounded_and_secrets_are_independent() -> None:
    settings = Settings()

    assert settings.email_verification_ttl_seconds == 600
    assert settings.email_verification_resend_seconds == 60
    assert settings.email_verification_max_attempts == 5
    assert settings.email_verification_ticket_ttl_seconds == 600
    assert settings.email_verification_source_window_seconds == 3600
    assert settings.email_verification_source_limit == 20
    assert isinstance(settings.email_verification_hmac_secret, SecretStr)
    assert str(settings.email_verification_hmac_secret) == "**********"

    with pytest.raises(ValueError):
        Settings.model_validate({"email_verification_max_attempts": 0})
    with pytest.raises(ValueError, match="must not reuse"):
        Settings.model_validate(
            {
                "jwt_secret_key": "same-secret-with-at-least-thirty-two-bytes",
                "email_verification_hmac_secret": ("same-secret-with-at-least-thirty-two-bytes"),
            }
        )


def test_production_smtp_requires_complete_secure_configuration() -> None:
    base = {
        "environment": "production",
        "jwt_secret_key": "production-jwt-secret-with-at-least-32-bytes",
        "email_verification_hmac_secret": ("production-registration-secret-with-at-least-32-bytes"),
        # Keep this behavior test independent from a developer's local SMTP profile.
        "smtp_host": None,
        "smtp_username": None,
        "smtp_password": None,
        "smtp_from_email": None,
    }

    with pytest.raises(ValueError, match="SMTP_HOST"):
        Settings.model_validate({**base, "smtp_enabled": True})

    configured = Settings.model_validate(
        {
            **base,
            "smtp_enabled": True,
            "smtp_host": "smtp.example.com",
            "smtp_port": 465,
            "smtp_tls_mode": "tls",
            "smtp_username": "mailer@example.com",
            "smtp_password": "smtp-production-secret",
            "smtp_from_email": "mailer@example.com",
            "smtp_from_name": "Lanverse",
        }
    )
    assert configured.smtp_enabled is True
    assert str(configured.smtp_password) == "**********"
