from __future__ import annotations

import pytest

from core.config import ApplicationSettings
from core.runtime import create_runtime


def minio_values() -> dict[str, object]:
    return {
        "MINIO_ENDPOINT": "127.0.0.1:9000",
        "MINIO_PUBLIC_ENDPOINT": "http://127.0.0.1:9000",
        "MINIO_BUCKET": "lanverse",
        "MINIO_ACCESS_KEY": "access-value",
        "MINIO_SECRET_KEY": "secret-value",
        "MINIO_SECURE": False,
    }


def test_complete_minio_configuration_builds_private_object_store() -> None:
    settings = ApplicationSettings.model_validate(minio_values())

    config = settings.require_minio_config()
    runtime = create_runtime(settings)

    assert config.endpoint == "127.0.0.1:9000"
    assert config.public_endpoint == "http://127.0.0.1:9000"
    assert config.bucket == "lanverse"
    assert config.secure is False
    assert "secret-value" not in repr(config)
    assert runtime.object_store is not None


def test_partial_or_invalid_minio_configuration_fails_with_variable_names_only() -> None:
    partial = ApplicationSettings.model_validate({"MINIO_ENDPOINT": "127.0.0.1:9000"})

    with pytest.raises(ValueError, match="MINIO_BUCKET") as missing:
        partial.require_minio_config()
    assert "secret-value" not in str(missing.value)

    invalid = minio_values()
    invalid["MINIO_ENDPOINT"] = "http://127.0.0.1:9000/path"
    with pytest.raises(ValueError, match="MINIO_ENDPOINT"):
        ApplicationSettings.model_validate(invalid).require_minio_config()
