import pytest
from pydantic import ValidationError

from app.config import Settings


def test_defaults(monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.delenv("LV_ENV", raising=False)
    monkeypatch.delenv("LV_LOG_LEVEL", raising=False)

    settings = Settings()

    assert settings.env == "local"
    assert settings.log_level == "info"


def test_reads_lv_prefixed_env(monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.setenv("LV_ENV", "staging")
    monkeypatch.setenv("LV_LOG_LEVEL", "debug")

    settings = Settings()

    assert settings.env == "staging"
    assert settings.log_level == "debug"


@pytest.mark.parametrize(("key", "value"), [("LV_ENV", "dev"), ("LV_LOG_LEVEL", "verbose")])
def test_rejects_invalid_values(monkeypatch: pytest.MonkeyPatch, key: str, value: str) -> None:
    monkeypatch.setenv(key, value)

    with pytest.raises(ValidationError):
        Settings()
