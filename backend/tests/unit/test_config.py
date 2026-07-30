from pathlib import Path

import pytest
from pydantic import SecretStr

from app.core.config import REPOSITORY_ENV_FILE, Settings
from app.core.database import validate_test_database_url

ROOT = Path(__file__).resolve().parents[3]


def test_settings_load_only_the_repository_environment_file() -> None:
    assert Settings.model_config.get("env_file") == REPOSITORY_ENV_FILE
    assert REPOSITORY_ENV_FILE == ROOT / ".env"


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


def test_deepseek_key_is_optional_and_secret() -> None:
    assert Settings.model_validate({}).deepseek_api_key is None
    assert Settings.model_validate({"deepseek_api_key": ""}).deepseek_api_key is None

    configured = Settings.model_validate({"deepseek_api_key": "test-deepseek-key"})

    assert isinstance(configured.deepseek_api_key, SecretStr)
    assert str(configured.deepseek_api_key) == "**********"
