import pytest

from app.creation.config import Settings, database_url
from tests.creation.test_contract import SECRET


def configure(monkeypatch: pytest.MonkeyPatch) -> None:
    for key in (
        "CREATION_TEMPORAL_ADDRESS",
        "CREATION_TEMPORAL_TLS",
        "CREATION_TEMPORAL_NAMESPACE",
        "CREATION_TASK_QUEUE",
        "AGENT_EXECUTION_SECRET",
    ):
        monkeypatch.delenv(key, raising=False)
    monkeypatch.setenv("CREATION_DATABASE_URL", "postgresql://creation@localhost/creation_test")
    monkeypatch.setenv("CREATION_AGENT_SECRET", SECRET)


def test_configuration_does_not_fall_back_to_platform_credentials(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    configure(monkeypatch)
    settings = Settings.from_environment()
    assert SECRET not in repr(settings)
    assert settings.database_url not in repr(settings)
    monkeypatch.delenv("CREATION_DATABASE_URL")
    monkeypatch.setenv("DATABASE_URL", "postgresql://platform@localhost/platform")
    with pytest.raises(ValueError, match="CREATION_DATABASE_URL"):
        database_url()


@pytest.mark.parametrize(
    ("key", "value"),
    [
        ("CREATION_AGENT_SECRET", "short"),
        ("CREATION_TEMPORAL_ADDRESS", "temporal.example.test:7233"),
        ("CREATION_TEMPORAL_NAMESPACE", ""),
        ("CREATION_TASK_QUEUE", ""),
        ("CREATION_TEMPORAL_TLS", "yes"),
    ],
)
def test_configuration_rejects_unsafe_or_empty_settings(
    monkeypatch: pytest.MonkeyPatch, key: str, value: str
) -> None:
    configure(monkeypatch)
    monkeypatch.setenv(key, value)
    with pytest.raises(ValueError):
        Settings.from_environment()


def test_configuration_requires_distinct_harness_key(monkeypatch: pytest.MonkeyPatch) -> None:
    configure(monkeypatch)
    monkeypatch.setenv("AGENT_EXECUTION_SECRET", SECRET)
    with pytest.raises(ValueError):
        Settings.from_environment()
