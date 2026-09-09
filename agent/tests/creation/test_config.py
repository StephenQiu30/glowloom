import pytest

from app.core.config import Settings, database_url
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


def test_worker_fails_fast_without_independent_trusted_endpoints(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    from app.core.config import WorkerSettings

    configure(monkeypatch)
    monkeypatch.setenv("CREATION_HARNESS_SECRET", "independent-harness-key-" * 3)
    monkeypatch.setenv("CREATION_TEXT_RELEASE_HASH", "a" * 64)
    monkeypatch.setenv("CREATION_PLATFORM_URL", "https://platform.test")
    monkeypatch.setenv("CREATION_HARNESS_URL", "http://127.0.0.1:8787")
    worker = WorkerSettings.from_environment()
    assert worker.call_limit == 1000
    assert worker.invocation_timeout_seconds == 600
    assert worker.harness_secret not in repr(worker)
    for key, value in [
        ("CREATION_PLATFORM_URL", "http://platform.test"),
        ("CREATION_PLATFORM_URL", "https://user:pass@platform.test"),
        ("CREATION_HARNESS_URL", "https://harness.test/arbitrary/path"),
        ("CREATION_HARNESS_URL", "https://harness.test?override=1"),
        ("CREATION_HARNESS_SECRET", SECRET),
        ("CREATION_TEXT_RELEASE_HASH", ""),
        ("CREATION_CALL_LIMIT", "1001"),
        ("CREATION_INVOCATION_TIMEOUT_SECONDS", "901"),
    ]:
        with monkeypatch.context() as changed:
            changed.setenv(key, value)
            with pytest.raises(ValueError):
                WorkerSettings.from_environment()


def test_docker_transport_is_explicit_and_limited_to_compose_peers(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    from app.core.config import trusted_url

    configure(monkeypatch)
    monkeypatch.setenv("CREATION_DOCKER_NETWORK", "true")
    for address in ("host.docker.internal:7233", "temporal:7233"):
        monkeypatch.setenv("CREATION_TEMPORAL_ADDRESS", address)
        assert Settings.from_environment().temporal_address == address
    monkeypatch.setenv("CREATION_TEMPORAL_ADDRESS", "untrusted-temporal:7233")
    with pytest.raises(ValueError):
        Settings.from_environment()
    monkeypatch.setenv("CREATION_TEMPORAL_ADDRESS", "temporal:7233")
    for key, value in [
        ("CREATION_PLATFORM_URL", "http://backend:8686"),
        ("CREATION_HARNESS_URL", "http://agent:8787"),
    ]:
        monkeypatch.setenv(key, value)
        assert trusted_url(key) == value
        monkeypatch.setenv(key, "http://remote.example:8686")
        with pytest.raises(ValueError):
            trusted_url(key)
    monkeypatch.setenv("CREATION_DOCKER_NETWORK", "false")
    with pytest.raises(ValueError):
        Settings.from_environment()
