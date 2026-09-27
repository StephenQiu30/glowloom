from pathlib import Path

import pytest
from pydantic import ValidationError

from app.main_worker import WorkerSettings


def _settings(**overrides: object) -> WorkerSettings:
    values: dict[str, object] = {
        "temporal_addr": "127.0.0.1:7233",
        "temporal_namespace": "lanverse-local",
        "redis_url": "redis://127.0.0.1:6379/0",
    }
    values.update(overrides)
    return WorkerSettings.model_validate(values)


def test_key_id_and_absolute_private_key_file_are_paired(tmp_path: Path) -> None:
    configured = _settings(
        credential_key_id="agent-test",
        credential_private_key_ref=tmp_path / "agent-private.pem",
    )
    assert configured.credential_key_id == "agent-test"
    assert configured.credential_private_key_ref == tmp_path / "agent-private.pem"

    invalid: list[dict[str, object]] = [
        {"credential_key_id": "agent-test"},
        {"credential_private_key_ref": tmp_path / "agent-private.pem"},
        {"credential_key_id": "agent-test", "credential_private_key_ref": Path("key.pem")},
    ]
    for values in invalid:
        with pytest.raises(ValidationError):
            _settings(**values)


def test_empty_optional_key_values_in_env_are_ignored(monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.setenv("LV_CREDENTIAL_KEY_ID", "")
    monkeypatch.setenv("LV_CREDENTIAL_PRIVATE_KEY_REF", "")
    configured = _settings()
    assert configured.credential_key_id is None
    assert configured.credential_private_key_ref is None


@pytest.mark.parametrize(
    "endpoint",
    [
        "ftp://127.0.0.1:4318",
        "http://user:private@127.0.0.1:4318",
        "http://127.0.0.1:4318/v1/traces",
        "http://127.0.0.1:4318?token=private",
        "http://127.0.0.1:4318/#fragment",
        "http://127.0.0.1:invalid",
    ],
)
def test_otel_endpoint_requires_http_origin_without_credentials(endpoint: str) -> None:
    with pytest.raises(ValidationError, match="invalid LV_OTEL_ENDPOINT"):
        _settings(otel_endpoint=endpoint)


def test_otel_endpoint_can_be_unset_or_http_origin(monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.setenv("LV_OTEL_ENDPOINT", "")
    assert _settings().otel_endpoint is None
    assert _settings(otel_endpoint="http://127.0.0.1:4318").otel_endpoint == (
        "http://127.0.0.1:4318"
    )
